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
	mcr := MailChimpRegistration{
		email,
		"subscribed",
	}
	json, err := json.Marshal(mcr)
	if err != nil {
		log.Println(err)
		return nil
	}

	client := &http.Client{}
	req, err := http.NewRequest(
		"POST",
		"http://us10.api.mailchimp.com/3.0/lists/ddf188e08e/members",
		strings.NewReader(string(json)),
	)
	if err != nil {
		log.Println(err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("agree", "b416eeb8b04228134e959d333675a950-us10")
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return nil
	}

	if resp.StatusCode != 200 {
		log.Println(resp)
		return nil
	}

	var wants_to_host int64
	if strings.ToLower(wants_to_host_s) == "true" {
		wants_to_host = 1
	} else {
		wants_to_host = 0
	}
	_, err = t.db.Exec(
		`INSERT INTO MealHost (Email, Will_host) VALUES (?, ?)`,
		email,
		wants_to_host,
	)
	if err != nil {
		log.Println(err)
		return nil
	}

	return APISuccess("OK")
}
