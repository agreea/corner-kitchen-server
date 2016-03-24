package main

import (
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strings"
)

type KitchenServlet struct {
	db              *sql.DB
	server_config   *Config
	session_manager *SessionManager
}

func NewKitchenServlet(server_config *Config, session_manager *SessionManager) *KitchenServlet {
	t := new(KitchenServlet)

	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewTruckServlet", "Failed to open database:", err)
	}
	t.db = db
	t.session_manager = session_manager

	return t
}

type MailChimpRegistration struct {
	Email_address string `json:"email_address"`
	Status        string `json:"status"`
}

func (t *KitchenServlet) Register(r *http.Request) *ApiResult {
	email := r.Form.Get("email")
	wants_to_host_s := r.Form.Get("host")
	var wants_to_host bool
	if wants_to_host_s == "true" {
		wants_to_host = true
	} else {
		wants_to_host = false
	}
	err := MailChimpRegister(email, wants_to_host, t.db)
	if err != nil {
		return APIError("Failed to register", 500)
	}
	return APISuccess("OK")
}

func MailChimpRegister(email string, wants_to_host bool, db *sql.DB) error {
	mcr := MailChimpRegistration{
		email,
		"subscribed",
	}
	log.Println(mcr)
	json, err := json.Marshal(mcr)
	if err != nil {
		log.Println(err)
		return err
	}
	client := &http.Client{}
	req, err := http.NewRequest(
		"POST",
		"http://us10.api.mailchimp.com/3.0/lists/ddf188e08e/members",
		strings.NewReader(string(json)),
	)
	if err != nil {
		log.Println(err)
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(server_config.Mailchimp.User, server_config.Mailchimp.Pass)
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return err
	}

	if resp.StatusCode != 200 {
		log.Println(resp)
		return err
	}

	var wants_to_host_val int64
	if wants_to_host {
		wants_to_host_val = 1
	} else {
		wants_to_host_val = 0
	}
	_, err = db.Exec(
		`INSERT INTO MealHost (Email, Will_host) VALUES (?, ?)`,
		email,
		wants_to_host_val,
	)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}