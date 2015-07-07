package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
)

type TruckServlet struct {
	db            *sql.DB
	server_config *Config
}

func NewTruckServlet(server_config *Config) *TruckServlet {
	t := new(TruckServlet)

	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewTruckServlet", "Failed to open database:", err)
	}
	t.db = db

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
