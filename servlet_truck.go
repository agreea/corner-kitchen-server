package main

import (
	"bitbucket.org/ckvist/twilio/twirest"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
)

type TruckServlet struct {
	db            *sql.DB
	server_config *Config
	twirest       *twirest.TwilioClient
}

func NewTruckServlet(server_config *Config) *TruckServlet {
	t := new(TruckServlet)

	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewTruckServlet", "Failed to open database:", err)
	}
	t.db = db

	t.twirest = twirest.NewClient(server_config.Twilio.SID, server_config.Twilio.Token)

	return t
}

func (t *TruckServlet) Find(r *http.Request) *ApiResult {
	lat_s := r.Form.Get("lat")
	lon_s := r.Form.Get("lon")
	radius_s := r.Form.Get("radius")

	lat, err := strconv.ParseFloat(lat_s, 64)
	if err != nil {
		return APIError("Malformed latitude", 400)
	}
	lon, err := strconv.ParseFloat(lon_s, 64)
	if err != nil {
		return APIError("Malformed longitude", 400)
	}
	radius, err := strconv.ParseFloat(radius_s, 64)
	if err != nil {
		return APIError("Malformed radius", 400)
	}

	trucks, err := GetTrucksNearLocation(t.db, lat, lon, radius)
	if err != nil {
		log.Println(err)
		return nil
	}
	return APISuccess(trucks)
}

func (t *TruckServlet) Message(r *http.Request) *ApiResult {
	message := r.Form.Get("message")
	to := r.Form.Get("number")

	msg := twirest.SendMessage{
		Text: message,
		To:   to,
		From: t.server_config.Twilio.From}
	resp, err := t.twirest.Request(msg)

	if err != nil {
		log.Println(err)
		return APIError(err.Error(), 500)
	}
	return APISuccess(resp.Message.Status)
}
func (t *TruckServlet) Menu(r *http.Request) *ApiResult {
	truck_id_s := r.Form.Get("truck_id")

	truck_id, err := strconv.ParseInt(truck_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed truck ID", 400)
	}

	menus, err := GetMenusForTruck(t.db, truck_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	return APISuccess(menus)
}
