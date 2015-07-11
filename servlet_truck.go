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
	db              *sql.DB
	server_config   *Config
	twilio_client   *twirest.TwilioClient
	session_manager *SessionManager
}

func NewTruckServlet(server_config *Config, session_manager *SessionManager, twilio_client *twirest.TwilioClient) *TruckServlet {
	t := new(TruckServlet)

	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewTruckServlet", "Failed to open database:", err)
	}
	t.db = db

	t.session_manager = session_manager

	t.twilio_client = twilio_client

	return t
}

func (t *TruckServlet) Find_truck(r *http.Request) *ApiResult {
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

func (t *TruckServlet) Find_food(r *http.Request) *ApiResult {
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

	food_list := make([]*MenuItem, 0)
	for _, truck := range trucks {
		for _, menu := range truck.Menus {
			food_list = append(food_list, menu.Items...)
		}
	}
	return APISuccess(food_list)
}

func (t *TruckServlet) Get_item(r *http.Request) *ApiResult {
	item_id_s := r.Form.Get("item_id")

	item_id, err := strconv.ParseInt(item_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return nil
	}

	item, err := GetMenuItemById(t.db, item_id)
	if err != nil {
		log.Println(err)
		return nil
	}

	return APISuccess(item)
}

func (t *TruckServlet) Message(r *http.Request) *ApiResult {
	message := r.Form.Get("message")
	to := r.Form.Get("number")

	msg := twirest.SendMessage{
		Text: message,
		To:   to,
		From: t.server_config.Twilio.From}
	resp, err := t.twilio_client.Request(msg)

	if err != nil {
		log.Println(err)
		return APIError(err.Error(), 500)
	}
	return APISuccess(resp.Message.Status)
}

func (t *TruckServlet) Order(r *http.Request) *ApiResult {
	//truck_id := r.Form.Get("truck_id")
	session_id := r.Form.Get("session")
	session_valid, _, err := t.session_manager.GetSession(session_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	if !session_valid {
		return APIError("Invalid session token", 401)
	}

	return APIError("Unimplemented", 400)
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
