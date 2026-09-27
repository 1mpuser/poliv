package main

import (
	"context"
	"fmt"
	"os"

	"poliv/internal/config"
	"poliv/internal/db"
	"poliv/internal/migrate"
	"poliv/internal/passwords"
	"poliv/internal/users"
)

func runCLI(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "нет подкоманды")
		return 1
	}
	cmd := args[0]
	flags := parseFlags(args[1:])

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	ctx := context.Background()
	dbConn, err := db.New(ctx, cfg.DatabaseURL, cfg.Zone)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		return 1
	}
	defer dbConn.Close()
	if err := migrate.New(dbConn.Pool).Up(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}

	email := flags["email"]
	password := flags["password"]
	if password == "" {
		password, _ = passwords.GeneratePassword(16)
	}

	switch cmd {
	case "set-owner":
		if email == "" {
			fmt.Fprintln(os.Stderr, "укажите --email")
			return 1
		}
		email = users.NormalizeEmail(email)
		user, _ := users.FindByEmail(ctx, dbConn.Pool, email)
		if user == nil {
			user, err = users.FindByEmail(ctx, dbConn.Pool, users.PlaceholderEmail)
		}
		if user == nil {
			user, err = users.CreateUser(ctx, dbConn.Pool, email, password, true)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		} else {
			if _, err := dbConn.Pool.Exec(ctx, `UPDATE users SET email=$1, is_admin=true, blocked_at=NULL WHERE id=$2`, email, user.ID); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			if err := users.SetPassword(ctx, dbConn.Pool, user, password); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		printCredentials(email, password)
	case "create-user":
		if email == "" {
			fmt.Fprintln(os.Stderr, "укажите --email")
			return 1
		}
		admin := flags["admin"] == "1"
		user, err := users.CreateUser(ctx, dbConn.Pool, email, password, admin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		printCredentials(user.Email, password)
	case "reset-password":
		if email == "" {
			fmt.Fprintln(os.Stderr, "укажите --email")
			return 1
		}
		user, err := users.FindByEmail(ctx, dbConn.Pool, email)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if user == nil {
			fmt.Fprintln(os.Stderr, "Учётка "+users.NormalizeEmail(email)+" не найдена")
			return 1
		}
		if err := users.SetPassword(ctx, dbConn.Pool, user, password); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		printCredentials(users.NormalizeEmail(email), password)
	case "make-admin":
		if email == "" {
			fmt.Fprintln(os.Stderr, "укажите --email")
			return 1
		}
		user, err := users.FindByEmail(ctx, dbConn.Pool, email)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if user == nil {
			fmt.Fprintln(os.Stderr, "Учётка "+users.NormalizeEmail(email)+" не найдена")
			return 1
		}
		if _, err := dbConn.Pool.Exec(ctx, `UPDATE users SET is_admin=true WHERE id=$1`, user.ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("%s теперь админ\n", user.Email)
	default:
		fmt.Fprintln(os.Stderr, "неизвестная подкоманда")
		return 1
	}
	return 0
}

func printCredentials(email, password string) {
	fmt.Printf("Почта:  %s\nПароль: %s\nСохраните пароль — повторно он не показывается.\n", email, password)
}

func parseFlags(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 2 && a[:2] == "--" {
			key := a[2:]
			if i+1 < len(args) && !(len(args[i+1]) > 2 && args[i+1][:2] == "--") {
				out[key] = args[i+1]
				i++
			} else {
				out[key] = "1"
			}
		}
	}
	return out
}
