package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
)

type Config struct{ Address, DatabaseURL, WebOrigin string }

func Load() (Config, error) {
	c := Config{Address: os.Getenv("API_ADDRESS"), DatabaseURL: os.Getenv("DATABASE_URL"), WebOrigin: os.Getenv("WEB_ORIGIN")}
	if c.Address == "" {
		c.Address = "127.0.0.1:8080"
	}
	_, port, err := net.SplitHostPort(c.Address)
	n, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || n < 1 || n > 65535 {
		return c, errors.New("API_ADDRESS must be a host:port address")
	}
	db, err := url.Parse(c.DatabaseURL)
	if err != nil || (db.Scheme != "postgres" && db.Scheme != "postgresql") || db.Host == "" {
		return c, errors.New("DATABASE_URL must be a PostgreSQL URL")
	}
	origin, err := url.Parse(c.WebOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return c, errors.New("WEB_ORIGIN must be an exact http(s) origin")
	}
	return c, nil
}
