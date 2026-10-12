package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	email := flag.String("email", "", "Initial admin email")
	name := flag.String("name", "", "Initial admin name")
	school := flag.String("school-name", "", "School name, if not yet created")
	flag.Parse()
	if err := run(*email, *name, *school); err != nil {
		fmt.Fprintln(os.Stderr, "Bootstrap failed; check input, migrations and whether an admin already exists.")
		os.Exit(1)
	}
	fmt.Println("Initial administrator created. No password was logged.")
}

func run(email, name, school string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	password, err := io.ReadAll(io.LimitReader(os.Stdin, 130))
	if err != nil {
		return err
	}
	if len(password) >= 130 {
		return errors.New("password too long")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	svc, err := auth.New(pool, cfg.SchoolID, cfg.WebOrigin)
	if err != nil {
		return err
	}
	return svc.Bootstrap(ctx, email, name, strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r"), school)
}
