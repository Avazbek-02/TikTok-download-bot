import sys
import subprocess

def get_download_url(video_url):
    """
    Berilgan Instagram yoki YouTube video URL uchun yuklab olish havolasini qaytaradi.
    """
    try:
        # `yt-dlp` orqali yuklab olish linkini olish
        result = subprocess.run(
            ["yt-dlp", "-g", video_url], capture_output=True, text=True
        )

        if result.returncode != 0:
            print(f"Xatolik: Yuklab olish linkini olishda muammo!\n{result.stderr}", file=sys.stderr)
            sys.exit(1)

        download_url = result.stdout.strip()
        return download_url

    except Exception as e:
        print(f"Xatolik: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Foydalanish: python downloader.py <video_url>", file=sys.stderr)
        sys.exit(1)

    video_url = sys.argv[1]
    
    if "instagram.com" in video_url or "youtube.com" in video_url or "youtu.be" in video_url:
        download_link = get_download_url(video_url)
        print("Yuklab olish havolasi:", download_link)
    else:
        print("Xatolik: Faqat YouTube va Instagram qo‘llab-quvvatlanadi!", file=sys.stderr)
        sys.exit(1)
