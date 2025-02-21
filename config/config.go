package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type (
	Config struct {
		TelegramApi `yaml:"telegramapi"`
	}

	TelegramApi struct {
		TelegramToken string `env-required:"true" yaml:"telegramtoken" env:"TELEGRAMTOKEN"`
	}
)

func NewConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("⚠️  Ogohlantirish: .env fayli topilmadi yoki yuklanmadi.")
	}

	cfg := &Config{}

	// // YAML fayldan o‘qish (agar mavjud bo‘lsa)
	// err = cleanenv.ReadConfig("./config/config.yml", cfg)
	// if err != nil {
	// 	fmt.Println("⚠️  Ogohlantirish: config.yml fayli topilmadi yoki o‘qib bo‘lmadi.")
	// }

	err = cleanenv.ReadEnv(cfg)
	if err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}

	return cfg, nil
}
