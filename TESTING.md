# Testing

## Manual smoke test

1. Start the server: `junkyard server`
2. In another terminal, upload a file: `junkyard send test.txt`
3. Note the short code displayed
4. Download the file: `junkyard receive <code>`
5. Verify the file contents match
6. Test resumable download: start download, Ctrl+C halfway, re-run receive with same code
7. Test multi-file: `junkyard send a.txt b.txt` (auto-zips)
8. Test --out flag: `junkyard receive <code> --out /tmp/downloads`

## Server curl one-liners

### Upload (single file)
```bash
curl -X POST -F "file=@localfile.txt" http://localhost:45845/upload
```
Expected response: `{"code":"A7B9X2","expires_in":600}`

### Upload (chunked)
```bash
# First chunk
curl -X POST -F "code=A7B9X2" -F "chunk_index=0" -F "total_chunks=3" -F "chunk=@chunk0.bin" http://localhost:45845/upload/chunk
# ... repeat for all chunks
```

### Download
```bash
curl http://localhost:45845/download/A7B9X2 > downloaded.txt
```

### Resumable download
```bash
curl -H "Range: bytes=1048576-" http://localhost:45845/download/A7B9X2 > downloaded.txt
```