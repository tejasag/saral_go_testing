# Build Stage
FROM golang:1.25-bookworm AS builder

# Install build dependencies
# We need these to compile OpenCV
RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    git \
    wget \
    unzip \
    pkg-config \
    libjpeg-dev \
    libpng-dev \
    libtiff-dev \
    libavcodec-dev \
    libavformat-dev \
    libswscale-dev \
    libv4l-dev \
    libxvidcore-dev \
    libx264-dev \
    libgtk-3-dev \
    libatlas-base-dev \
    gfortran \
    python3-dev \
    libmupdf-dev \
    && rm -rf /var/lib/apt/lists/*

# Install ONNX Runtime
WORKDIR /tmp
RUN wget -q https://github.com/microsoft/onnxruntime/releases/download/v1.16.3/onnxruntime-linux-x64-1.16.3.tgz \
    && tar -zxf onnxruntime-linux-x64-1.16.3.tgz \
    && cp onnxruntime-linux-x64-1.16.3/lib/libonnxruntime.so.1.16.3 /usr/lib/libonnxruntime.so \
    && cp onnxruntime-linux-x64-1.16.3/lib/libonnxruntime.so.1.16.3 /usr/lib/libonnxruntime.so.1.16.3

WORKDIR /app

# Copy dependency definitions
COPY go.mod go.sum ./
RUN go mod download

# Install OpenCV 4.10.0 using GoCV helper script
# This is necessary because Debian Bookworm has older OpenCV (4.6.0) which is incompatible with GoCV v0.43.0
RUN cd /go/pkg/mod/gocv.io/x/gocv@v0.43.0 && \
    make install_4.10.0

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=1 is required for GoCV and go-fitz
RUN CGO_ENABLED=1 GOOS=linux go build -o main .

# Runtime Stage
FROM debian:bookworm-slim

# Install Runtime Dependencies
# We need to copy the compiled OpenCV libraries from the builder stage
# or install dependencies that the compiled libraries might need.
RUN apt-get update && apt-get install -y \
    ffmpeg \
    texlive-latex-base \
    texlive-latex-extra \
    texlive-fonts-recommended \
    libmupdf-dev \
    ca-certificates \
    # Runtime deps for OpenCV (often needed even if we copy libs)
    libjpeg62-turbo \
    libpng16-16 \
    libtiff6 \
    libavcodec59 \
    libavformat59 \
    libswscale6 \
    libv4l-0 \
    libxvidcore4 \
    libx264-164 \
    libgtk-3-0 \
    libatlas3-base \
    && rm -rf /var/lib/apt/lists/*

# Copy ONNX Runtime libraries
COPY --from=builder /usr/lib/libonnxruntime.so /usr/lib/libonnxruntime.so
COPY --from=builder /usr/lib/libonnxruntime.so.1.16.3 /usr/lib/libonnxruntime.so.1.16.3

# Copy Compiled OpenCV libraries from /usr/local/lib
# The script usually installs to /usr/local/lib or /usr/local/lib64
COPY --from=builder /usr/local/lib/libopencv* /usr/local/lib/
COPY --from=builder /usr/local/include/opencv4 /usr/local/include/opencv4

# Update library cache
RUN ldconfig

# Copy binary
COPY --from=builder /app/main /app/main

# Copy Assets and ONNX Model
COPY --from=builder /app/assets /app/assets
COPY --from=builder /app/yolov8n-doclaynet.onnx /app/yolov8n-doclaynet.onnx

WORKDIR /app

# Expose port
ENV PORT=8080
EXPOSE 8080

# Run Server
CMD ["./main", "--server", "--port=:8080"]
