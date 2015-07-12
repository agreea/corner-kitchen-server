package main

import (
	"bitbucket.org/ckvist/twilio/twirest"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"time"
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
	open_from_unix_s := r.Form.Get("open_from")
	open_til_unix_s := r.Form.Get("open_til")

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
	open_til_unix, err := strconv.ParseInt(open_til_unix_s, 10, 64)
	if err != nil {
		return APIError("Malformed open to time", 400)
	}
	open_til := time.Unix(open_til_unix, 0)

	open_from_unix, err := strconv.ParseInt(open_from_unix_s, 10, 64)
	if err != nil {
		return APIError("Malformed open from time", 400)
	}
	open_from := time.Unix(open_from_unix, 0)

	trucks, err := GetTrucksNearLocation(t.db, lat, lon, radius, open_from, open_til)
	if err != nil {
		log.Println(err)
		return nil
	}
	return APISuccess(trucks)
}

func (t *TruckServlet) Open_up(r *http.Request) *ApiResult {
	lat_s := r.Form.Get("lat")
	lon_s := r.Form.Get("lon")
	truck_id_s := r.Form.Get("truck_id")
	open_from_unix_s := r.Form.Get("open_from")
	open_til_unix_s := r.Form.Get("open_til")

	lat, err := strconv.ParseFloat(lat_s, 64)
	if err != nil {
		return APIError("Malformed latitude", 400)
	}

	lon, err := strconv.ParseFloat(lon_s, 64)
	if err != nil {
		return APIError("Malformed longitude", 400)
	}

	truck_id, err := strconv.ParseInt(truck_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed truck ID", 400)
	}

	open_til_unix, err := strconv.ParseInt(open_til_unix_s, 10, 64)
	if err != nil {
		return APIError("Malformed open to time", 400)
	}
	open_til := time.Unix(open_til_unix, 0)

	open_from_unix, err := strconv.ParseInt(open_from_unix_s, 10, 64)
	if err != nil {
		return APIError("Malformed open from time", 400)
	}
	open_from := time.Unix(open_from_unix, 0)

	_, err = t.db.Exec(`
		UPDATE Truck SET
		Location_lat = ?,
		Location_lon = ?,
		Open_from = ?,
		Open_until = ?
		WHERE Id = ?`, lat, lon, open_from, open_til, truck_id)
	if err != nil {
		log.Println(err)
		return nil
	}

	return APISuccess("OK")
}

func (t *TruckServlet) Find_food(r *http.Request) *ApiResult {
	lat_s := r.Form.Get("lat")
	lon_s := r.Form.Get("lon")
	radius_s := r.Form.Get("radius")
	open_from_unix_s := r.Form.Get("open_from")
	open_til_unix_s := r.Form.Get("open_til")

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
	open_til_unix, err := strconv.ParseInt(open_til_unix_s, 10, 64)
	if err != nil {
		return APIError("Malformed open to time", 400)
	}
	open_til := time.Unix(open_til_unix, 0)

	open_from_unix, err := strconv.ParseInt(open_from_unix_s, 10, 64)
	if err != nil {
		return APIError("Malformed open from time", 400)
	}
	open_from := time.Unix(open_from_unix, 0)

	trucks, err := GetTrucksNearLocation(t.db, lat, lon, radius, open_from, open_til)
	if err != nil {
		log.Println(err)
		return nil
	}

	food_list := make([]*MenuItem, 0)
	for _, truck := range trucks {
		for _, menu := range truck.Menus {
			if menu.Flagship {
				food_list = append(food_list, menu.Items...)
			}
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
