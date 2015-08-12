package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"time"
)

type TruckServlet struct {
	db              *sql.DB
	server_config   *Config
	twilio_queue    chan *SMS
	session_manager *SessionManager
}

func NewTruckServlet(server_config *Config, session_manager *SessionManager, twilio_queue chan *SMS) *TruckServlet {
	t := new(TruckServlet)

	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewTruckServlet", "Failed to open database:", err)
	}
	t.db = db

	t.session_manager = session_manager

	t.twilio_queue = twilio_queue

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

func (t *TruckServlet) Close_down(r *http.Request) *ApiResult {
	truck_id_s := r.Form.Get("truck_id")
	session_id := r.Form.Get("session")

	truck_id, err := strconv.ParseInt(truck_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed truck ID", 400)
	}

	session_valid, session, err := t.session_manager.GetSession(session_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	if !session_valid {
		return APIError("Invalid session token", 401)
	}

	truck, err := GetTruckById(t.db, truck_id)
	if err != nil {
		log.Println(err)
		return nil
	}

	if session.User.Id != truck.Owner {
		return APIError("Not autorized to open for truck", 401)
	}

	_, err = t.db.Exec(`
		UPDATE Truck SET
		Open_until = ?
		WHERE Id = ?`, time.Now(), truck_id)
	if err != nil {
		log.Println(err)
		return nil
	}

	if err != nil {
		log.Println(err)
		return nil
	}

	return APISuccess("OK")
}

func (t *TruckServlet) Open_up(r *http.Request) *ApiResult {
	lat_s := r.Form.Get("lat")
	lon_s := r.Form.Get("lon")
	truck_id_s := r.Form.Get("truck_id")
	open_from_unix_s := r.Form.Get("open_from")
	open_til_unix_s := r.Form.Get("open_til")
	session_id := r.Form.Get("session")

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

	session_valid, session, err := t.session_manager.GetSession(session_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	if !session_valid {
		return APIError("Invalid session token", 401)
	}

	truck, err := GetTruckById(t.db, truck_id)
	if err != nil {
		log.Println(err)
		return nil
	}

	if session.User.Id != truck.Owner {
		return APIError("Not autorized to open for truck", 401)
	}

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

type OrderJson []OrderJsonItem
type OrderJsonItem struct {
	Id            int64   `json:"item_id"`
	Quantity      int64   `json:quantity`
	ListOptions   []int64 `json:"listoptions"`
	ToggleOptions []int64 `json:"toggleoptions"`
}

func (t *TruckServlet) Order(r *http.Request) *ApiResult {
	truck_id_s := r.Form.Get("truck_id")
	truck_id, err := strconv.ParseInt(truck_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed truck ID", 400)
	}

	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetSession(session_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	if !session_valid {
		return APIError("Invalid session token", 401)
	}

	// Check if the truck is still open
	truck, err := GetTruckById(t.db, truck_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	if truck.Open_until.Before(time.Now()) {
		return APIError("This truck is currently closed and not accepting orders", 400)
	}

	items_json := r.Form.Get("items")
	log.Println(items_json)
	var order_body OrderJson
	err = json.Unmarshal([]byte(items_json), &order_body)
	if err != nil {
		log.Println(err)
		return nil
	}
	log.Println(order_body)

	// Generate order
	order := new(Order)
	order.User_id = session.User.Id
	order.Truck_id = truck_id
	order.Date = time.Now()

	pickup_time_s := r.Form.Get("pickup_time")
	pickup_time, err := strconv.ParseInt(pickup_time_s, 10, 64)
	if err != nil {
		return APIError("Invalid pickup time", 400)
	}
	order.Pickup_time = time.Unix(pickup_time, 0)

	// Collect full data on items
	order_items := make([]*OrderItem, len(order_body))
	for i, item := range order_body {
		orderitem := new(OrderItem)
		orderitem.Order_id = order.Id
		orderitem.Item_id = item.Id
		orderitem.Quantity = item.Quantity

		orderitem.ToggleOptions = item.ToggleOptions
		orderitem.ListOptionValues = item.ListOptions

		order_items[i] = orderitem
	}

	order.Items = order_items

	// Save the order to the DB
	err = SaveOrderToDB(t.db, order)
	if err != nil {
		log.Println(err)
		return nil
	}

	// Send user notification
	msg := new(SMS)
	msg.To = session.User.Phone
	order_text := ""
	for _, item := range order_body {
		mitem, err := GetMenuItemById(t.db, item.Id)
		if err != nil {
			log.Println(err)
			return nil
		}
		if len(order_text) == 0 {
			order_text = mitem.Name
		} else {
			order_text = fmt.Sprintf("%s, %s", order_text, mitem.Name)
		}
	}
	msg.Message = fmt.Sprintf("Your order (%s) has been placed!", order_text)
	t.twilio_queue <- msg

	// Send truck notification
	msg = new(SMS)
	msg.To = truck.Phone
	full_order_text := ""
	for _, item := range order_body {
		mitem, err := GetMenuItemById(t.db, item.Id)

		if err != nil {
			log.Println(err)
			return nil
		}

		item_desc := ""
		for _, listopt := range item.ListOptions {
			opt, err := GetListOptionValueById(t.db, listopt)
			if err != nil {
				log.Println(err)
				return nil
			}
			if len(item_desc) == 0 {
				item_desc = fmt.Sprintf("%s", opt.Option_name)
			} else {
				item_desc = fmt.Sprintf("%s, %s", item_desc, opt.Option_name)
			}
		}

		for _, toggleopt := range item.ToggleOptions {
			opt, err := GetToggleOptionById(t.db, toggleopt)
			if err != nil {
				log.Println(err)
				return nil
			}
			if len(item_desc) == 0 {
				item_desc = fmt.Sprintf("%s", opt.Name)
			} else {
				item_desc = fmt.Sprintf("%s, %s", item_desc, opt.Name)
			}
		}

		if len(order_text) == 0 {
			full_order_text = fmt.Sprintf("%s (%s)", mitem.Name, item_desc)
		} else {
			full_order_text = fmt.Sprintf("%s, %s (%s)", full_order_text, mitem.Name, item_desc)
		}
	}
	msg.Message = fmt.Sprintf(
		"%s wants: %s, pickup: %s",
		session.User.First_name,
		full_order_text,
		order.Pickup_time,
	)
	t.twilio_queue <- msg

	return APISuccess("OK")
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
