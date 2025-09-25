package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
	webhook := os.Getenv("DISCORD_WEBHOOK")
	if webhook == "" {
		log.Fatal("Webhook not set")
	}
	db, err := sql.Open("sqlite3", "file:data.db?mode=ro&_busy_timeout=5000&_foreign_keys=on&_journal_mode=wal")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err.Error())
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			log.Fatalf("Failed to close database: %v", err.Error())
		}
	}(db)

	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	sqlCommand := fmt.Sprintf(`
	SELECT
	  u.name AS "Name",
	  COUNT(s.user_id) AS "Total Solves"
	FROM users u
	LEFT JOIN solves s
	  ON s.user_id = u.id
	 AND s.solved_at_unix >= strftime('%%s', '%s 00:30:00', 'utc')
	 AND s.solved_at_unix <  strftime('%%s', date('%s', '+1 day') || ' 00:30:00', 'utc')
	WHERE u.actively_tracking = 1
	GROUP BY u.id, u.name
	ORDER BY "Total Solves" DESC, u.name;
	`, date, date)

	rows, err := db.Query(sqlCommand)
	if err != nil {
		log.Fatalf("Failed to query database: %v", err.Error())
	}

	content := fmt.Sprintf("CSES Daily Stats Backup Completed\nDate: %s\n\n", date)

	for rows.Next() {
		var name, solves string
		err = rows.Scan(&name, &solves)
		if err != nil {
			log.Fatalf("Failed to scan row: %v", err.Error())
		}
		content = content + name + ": " + solves + "\n"
	}

	resp, err := http.PostForm(webhook, url.Values{
		"content": {content},
	})
	if err != nil {
		log.Fatalf("Failed to post webhook: %v", err.Error())
	}
	err = resp.Body.Close()
	if err != nil {
		log.Fatalf("Failed to close body: %v", err.Error())
	}
}
