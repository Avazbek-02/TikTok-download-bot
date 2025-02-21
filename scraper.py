import os
import time
import asyncio
import logging
from pyrogram import Client, filters
from playwright.async_api import async_playwright
from pytube import YouTube

# Telegram bot sozlamalari
API_ID = 28127771
API_HASH = "9a22b1950d6f05d4092c262fec898d78" 
BOT_TOKEN = "7980716216:AAHxMXTLVw3obzu9ZsphJPScQB1jR7JncIo"

app = Client("downloader_bot", api_id=API_ID, api_hash=API_HASH, bot_token=BOT_TOKEN)

# Foydalanuvchi cheklovlarini nazorat qilish uchun
user_requests = {}
REQUEST_LIMIT = 3  # Maksimal so‘rovlar soni
RATE_LIMIT_WINDOW = 5 * 60  # Cheklash oynasi (soniyalar)
BAN_TIME = 60 * 30  # Bloklash vaqti (soniyalar)
banned_users = {}

# Instagram videoni yuklab olish
async def download_instagram(video_url):
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True)
        page = await browser.new_page()
        await page.goto("https://igram.io/", timeout=60000)

        await page.fill("input[name='url']", video_url)
        await page.click("button[type='submit']")
        await asyncio.sleep(5)

        download_links = await page.query_selector_all("a")

        if not download_links:
            await browser.close()
            return None

        download_url = await download_links[0].get_attribute("href")
        await browser.close()
        return download_url

# YouTube videoni yuklab olish
async def download_youtube(video_url):
    yt = YouTube(video_url)
    stream = yt.streams.filter(progressive=True, file_extension="mp4").first()
    return stream.url if stream else None

# Rate limiting tekshirish
def is_rate_limited(user_id):
    now = time.time()

    if user_id in banned_users and now < banned_users[user_id]:
        return True

    if user_id not in user_requests:
        user_requests[user_id] = []

    user_requests[user_id] = [t for t in user_requests[user_id] if now - t < RATE_LIMIT_WINDOW]

    if len(user_requests[user_id]) >= REQUEST_LIMIT:
        banned_users[user_id] = now + BAN_TIME
        del user_requests[user_id]
        return True

    user_requests[user_id].append(now)
    return False

# Start komandasi
@app.on_message(filters.command("start"))
async def start(client, message):
    await message.reply_text("🎉 Assalomu alaykum! Media Download botiga xush kelibsiz!\n\n"
                             "📱 Men sizga quyidagi xizmatlarni taqdim etaman:\n"
                             "- YouTube video/audio\n"
                             "- Instagram post/reels\n\n"
                             "🔍 Ishlash tartibi:\n"
                             "1. Yuklab olmoqchi bo'lgan link/URL ni yuboring\n"
                             "2. Men sizga faylni yuklab beraman!\n\n"
                             "⚡️ Tezkor va ishonchli xizmat!\n\n"
                             "🤖 Bot @media_download_any_bot orqali ishlaydi!")

# URL qabul qilish va yuklab olish
@app.on_message(filters.text)
async def handle_download(client, message):
    user_id = message.from_user.id
    url = message.text.strip()

    if is_rate_limited(user_id):
        await message.reply_text("⚠️ Siz vaqtinchalik bloklandingiz yoki juda ko‘p so‘rov yubordingiz. Keyinroq urinib ko‘ring.")
        return

    if "instagram.com" in url:
        status_msg = await message.reply_text("🔄 Instagram videosi yuklanmoqda...")
        video_url = await download_instagram(url)
    elif "youtube.com" in url or "youtu.be" in url:
        status_msg = await message.reply_text("🔄 YouTube videosi yuklanmoqda...")
        video_url = await download_youtube(url)
    else:
        await message.reply_text("❌ Iltimos, Instagram yoki YouTube URL yuboring!")
        return

    if not video_url:
        await message.reply_text("❌ Xatolik: video yuklab olinmadi.")
        return

    await status_msg.edit("✅ Yuklab olish tayyor! Yuborilmoqda...")

    await message.reply_video(video_url, caption="✨ @media_download_any_bot orqali yuklab olindi")

# Botni ishga tushirish
if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    print("Bot ishga tushdi! 🚀")
    app.run()
