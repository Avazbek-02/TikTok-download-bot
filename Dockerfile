# Python 3.10 asosida imidj yaratamiz
FROM python:3.10

# Ishchi katalogni yaratamiz
WORKDIR /app

# Kerakli paketlarni o‘rnatamiz
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Playwright brauzerlarini o‘rnatamiz
RUN playwright install --with-deps

# Bot skriptini konteynerga nusxalaymiz
COPY . .

# Botni ishga tushiramiz
CMD ["python", "scraper.py"]
