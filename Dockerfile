# 1. Go image'dan foydalanamiz
FROM golang:1.22.1 AS builder

# 2. Ishchi katalog yaratamiz
WORKDIR /app

# 3. Modul va kodlarni nusxalash
COPY go.mod go.sum ./
RUN go mod download

# 4. Butun kodni konteynerga nusxalash
COPY . .

# 5. Go kodni kompilyatsiya qilish
RUN go build -o main .

# 6. Yangi toza image yaratamiz
FROM golang:1.22.1

# 7. Ishchi katalogni o‘rnatamiz
WORKDIR /app

# 8. Fayllarni nusxalash
COPY --from=builder /app/main .
COPY --from=builder /app/config/config.yml ./config/config.yml

# 9. .env faylini nusxalash
COPY .env .env

# 10. Port ochish
EXPOSE 8080

# 11. Botni ishga tushirish
CMD ["./main"]
