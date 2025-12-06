package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	ServerPort string

	JWTSecret string

	AccessTokenMaxAge  int
	RefreshTokenMaxAge int

	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
	R2PublicURL       string

	DefaultAvatarURL string
	DefaultAvatarKey string
}

func LoadConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found or error loading it, relying on environment variables")
	}

	accessTokenMaxAge, err := strconv.Atoi(os.Getenv("ACCESS_TOKEN_MAX_AGE"))
	if err != nil || accessTokenMaxAge <= 0 {
		accessTokenMaxAge = 900
	}

	refreshTokenMaxAge, err := strconv.Atoi(os.Getenv("REFRESH_TOKEN_MAX_AGE"))
	if err != nil || refreshTokenMaxAge <= 0 {
		refreshTokenMaxAge = 2592000
	}

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "8080"
	}

	r2AccountID := os.Getenv("R2_ACCOUNT_ID")
	r2AccessKeyID := os.Getenv("R2_ACCESS_KEY_ID")
	r2SecretAccessKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	r2BucketName := os.Getenv("R2_BUCKET_NAME")
	r2PublicURL := os.Getenv("R2_PUBLIC_URL")

	defaultAvatarURL := os.Getenv("DEFAULT_AVATAR_URL")
	defaultAvatarKey := os.Getenv("DEFAULT_AVATAR_KEY")

	return &Config{
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),

		ServerPort: serverPort,

		JWTSecret: os.Getenv("JWT_SECRET"),

		AccessTokenMaxAge:  accessTokenMaxAge,
		RefreshTokenMaxAge: refreshTokenMaxAge,

		R2AccountID:       r2AccountID,
		R2AccessKeyID:     r2AccessKeyID,
		R2SecretAccessKey: r2SecretAccessKey,
		R2BucketName:      r2BucketName,
		R2PublicURL:       r2PublicURL,

		DefaultAvatarURL: defaultAvatarURL,
		DefaultAvatarKey: defaultAvatarKey,
	}, nil
}
