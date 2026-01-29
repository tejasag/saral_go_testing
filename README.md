# `saral_go_testing`

## Setup

- Clone
- Run `go mod tidy`
- Run `go run . --server` (more options in main.go)

## External Dependencies

- FFmpeg
- FFprobe (usually installed with FFmpeg)
- PDFLatex 
- ONNX Runtime (using brew on Mac)
  - Verify `/opt/homebrew/lib/libonnxruntime.dylib` exists on Mac or `/usr/lib/libonnxruntime.so` exists on Linux

## Sample `.env` file

```
GEMINI_API_KEY=
SARVAM_API_KEY=
```

## API:

- POST `<any-route>?mode=video|poster` - Upload PDF via `pdf` form field
- GET `/status?id=<job_id>` - Check job status
- GET `/health` - Server health + queue info
