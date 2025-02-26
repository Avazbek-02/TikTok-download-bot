# Golang asosidagi imijdan foydalanamiz
FROM golang:1.22.1

# Ishchi katalogni yaratamiz
WORKDIR /app

# Kerakli paketlarni o‘rnatamiz
RUN apt-get update && apt-get install -y \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# yt-dlp ni yuklab olamiz
RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp

# Go mod va source kodni ko‘chiramiz
COPY go.mod go.sum ./
RUN go mod download

# Source kodni ko‘chiramiz
COPY . .

# Botni build qilamiz
RUN go build -o bot .

# Botni ishga tushiramiz
CMD ["go", "run", "main.go"]
