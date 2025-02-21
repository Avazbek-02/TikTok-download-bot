# 1-qadam: Bazaviy image
FROM golang:1.22.1 AS builder

WORKDIR /app

# Modul fayllarni ko‘chirish
COPY go.mod go.sum ./
RUN go mod download

# Ilova kodini ko‘chirish
COPY . .


# Ilovani qurish
RUN go build -o bot .

# 2-qadam: Asosiy image
FROM debian:bookworm-slim

WORKDIR /app

# 🔥 FFMPEG va YT-DLP o‘rnatamiz
RUN apt update && apt install -y \
    ffmpeg \
    wget \
    python3 \
    python3-pip && \
    wget https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -O /usr/local/bin/yt-dlp && \
    chmod a+rx /usr/local/bin/yt-dlp

# Binary faylni ko‘chirish
COPY --from=builder /app/bot .

# Botni ishga tushirish
CMD ["./bot"]
