package main

import (
	"database/sql"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
	// Create http client
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Failed to create cookie jar: %v", err.Error())
	}
	client := &http.Client{
		Jar: jar,
	}

	// handle login get
	var csrf string
	{
		resp, err := client.Get("https://cses.fi/login")
		if err != nil {
			log.Fatalf("Failed to get login page: %v", err.Error())
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Failed to read body (Login GET): %v", err.Error())
		}
		err = resp.Body.Close()
		if err != nil {
			log.Fatalf("Failed to close body (Login GET): %v", err.Error())
		}
		if resp.StatusCode != 200 {
			log.Fatalf("Login fetch failed with status code: %v\n and body:\n %v", resp.StatusCode, body)
		}
		re := regexp.MustCompile(`name="csrf_token"\s+value="([^"]+)"`)
		matches := re.FindStringSubmatch(string(body))
		if len(matches) < 2 {
			log.Fatalf("Failed to find CSRF token in login page")
		}
		csrf = matches[1]
	}

	// handle login post
	{
		nick := os.Getenv("CSES_NICK")
		if nick == "" {
			log.Fatal("CSES_NICK not set")
		}
		pass := os.Getenv("CSES_PASS")
		if pass == "" {
			log.Fatal("CSES_PASS not set")
		}
		resp, err := client.PostForm("https://cses.fi/login", url.Values{
			"csrf_token": {csrf},
			"nick":       {nick},
			"pass":       {pass},
		})
		if err != nil {
			log.Fatalf("Failed to post login: %v", err.Error())
		}
		err = resp.Body.Close()
		if err != nil {
			log.Fatalf("Failed to close body (Login POST): %v", err.Error())
		}
	}

	db, err := sql.Open("sqlite3", "file:data.db?mode=rw&_busy_timeout=5000&_foreign_keys=on&_journal_mode=wal")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err.Error())
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			log.Fatalf("Failed to close database: %v", err.Error())
		}
	}(db)

	type user struct {
		userIDint int
		userID    string
	}

	var userIDlist []user

	// handle creating userIDlist
	{
		rows, err := db.Query("SELECT id,cses_handle_id FROM users WHERE actively_tracking is TRUE")
		if err != nil {
			log.Fatalf("Failed to query database: %v", err.Error())
		}
		for rows.Next() {
			var currUser user
			err = rows.Scan(&currUser.userIDint, &currUser.userID)
			if err != nil {
				log.Fatalf("Failed to scan row: %v", err.Error())
			}
			userIDlist = append(userIDlist, currUser)
		}
		err = rows.Close()
		if err != nil {
			log.Fatalf("Failed to close rows: %v", err.Error())
		}
	}

	// fetch user stats
	re := regexp.MustCompile(`<td[\s\S]*?/task/(\d+)[\s\S]*?class="(?:[^"]*?(full))?[\s\S]*?</td>`)
	for _, currUser := range userIDlist {
		var userIDint int = currUser.userIDint
		var userID string = currUser.userID
		resp, err := client.Get("https://cses.fi/problemset/user/" + userID)
		if err != nil {
			log.Fatalf("Failed to get user page for user %s: %v", userID, err.Error())
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Failed to read body (User %s): %v", userID, err.Error())
		}
		err = resp.Body.Close()
		if err != nil {
			log.Fatalf("Failed to close body (User %s): %v", userID, err.Error())
		}
		if resp.StatusCode != 200 {
			log.Fatalf("User fetch failed for user %s with status code: %v\n and body:\n %v", userID, resp.StatusCode, body)
		}
		var matches [][]string = re.FindAllStringSubmatch(string(body), -1)
		for _, match := range matches {
			var problemID string = match[1]
			var full bool = match[2] == "full"
			if !full {
				continue
			}
			var problemIDint int
			err = db.QueryRow("SELECT id FROM problems WHERE cses_problem_id = ?", problemID).Scan(&problemIDint)
			if err != nil {
				log.Fatalf("Failed to find problem %v in database: %v", problemID, err.Error())
			}
			// check if previously solved
			var exists bool
			err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM solves WHERE user_id = ? AND problem_id = ?)", userIDint, problemIDint).Scan(&exists)
			if err != nil {
				log.Fatalf("Failed to check if problem %v is already solved for user %v: %v", problemID, userID, err.Error())
			}
			if exists {
				continue
			}
			_, err = db.Exec("INSERT INTO solves (user_id, problem_id) VALUES (?, ?)", userIDint, problemIDint)
			if err != nil {
				log.Fatalf("Failed to insert solved problem %v for user %v: %v", problemID, userID, err.Error())
			}
		}
	}
}
