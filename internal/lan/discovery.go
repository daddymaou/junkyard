package lan

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	Port       = 45845
	MagicBytes = 0x4A554E4B // "JUNK"
	Version    = 1
)

type DiscoveryMsg struct {
	Magic   uint32
	Version uint8
	Code    string
	Addr    string
}

func (m *DiscoveryMsg) Marshal() []byte {
	codeBytes := []byte(m.Code)
	addrBytes := []byte(m.Addr)
	buf := make([]byte, 5+1+len(codeBytes)+1+len(addrBytes))
	binary.BigEndian.PutUint32(buf[0:4], m.Magic)
	buf[4] = m.Version
	buf[5] = byte(len(codeBytes))
	copy(buf[6:6+len(codeBytes)], codeBytes)
	offset := 6 + len(codeBytes)
	buf[offset] = byte(len(addrBytes))
	copy(buf[offset+1:], addrBytes)
	return buf
}

func Unmarshal(data []byte) (*DiscoveryMsg, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("too short")
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != MagicBytes {
		return nil, fmt.Errorf("invalid magic")
	}
	version := data[4]
	if version != Version {
		return nil, fmt.Errorf("unsupported version")
	}
	codeLen := int(data[5])
	if len(data) < 6+codeLen+1 {
		return nil, fmt.Errorf("invalid code length")
	}
	code := string(data[6 : 6+codeLen])
	offset := 6 + codeLen
	addrLen := int(data[offset])
	if len(data) < offset+1+addrLen {
		return nil, fmt.Errorf("invalid addr length")
	}
	addr := string(data[offset+1 : offset+1+addrLen])
	if offset+1+addrLen != len(data) {
		return nil, fmt.Errorf("unexpected trailing data")
	}
	return &DiscoveryMsg{
		Magic:   magic,
		Version: version,
		Code:    code,
		Addr:    addr,
	}, nil
}

func BroadcastListener(code string, handler func(string, string)) error {
	return ListenDiscovery(nil, func(foundCode, address string) {
		if foundCode == code {
			handler(foundCode, address)
		}
	})
}

func ListenDiscovery(done <-chan struct{}, handler func(string, string)) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: Port})
	if err != nil {
		return err
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		if done != nil {
			select {
			case <-done:
				return nil
			default:
			}
		}
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if done != nil {
				select {
				case <-done:
					return nil
				default:
				}
			}
			return err
		}
		msg, err := Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		address := strings.TrimSpace(msg.Addr)
		if _, _, err := net.SplitHostPort(address); err != nil {
			address = remoteAddr.String()
		}
		handler(msg.Code, address)
	}
}

func LocalIPv4() (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip != nil && !ip.IsLoopback() {
				return ip, nil
			}
		}
	}
	return nil, fmt.Errorf("no active IPv4 address found")
}

func SendDiscovery(code, localAddr string) error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", Port))
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	msg := &DiscoveryMsg{
		Magic:   MagicBytes,
		Version: Version,
		Code:    code,
		Addr:    localAddr,
	}
	_, err = conn.Write(msg.Marshal())
	return err
}
