package main

import (
	"database/sql"
	"time"
)

/*
 * Trucks and meuns
 */

type Truck struct {
	// Raw fields
	Id           int64
	Name         string
	Location_lat string
	Location_lon string
	Open_from    time.Time
	Open_until   time.Time
	Phone        string
	Description  string

	// Go fields
	Distance float64
	Menus    []*Menu
	Open_now bool
}

type Menu struct {
	// Raw fields
	Id          int64
	Truck_id    int64
	Name        string
	Description string
	Flagship    bool

	// Go fields
	Items []*MenuItem
}

type MenuItem struct {
	// Raw fields
	Id          int64
	Truck_id    int64
	Menu_id     int64
	Name        string
	Price       float64
	Description string

	ListOptions   []*MenuItemOption
	ToggleOptions []*MenuToggleOption
}

type MenuToggleOption struct {
	Id             int64
	Item_id        int64
	Name           string
	Price_modifier float64
}

type MenuItemOption struct {
	Id      int64
	Item_id int64
	Name    string

	Values []*MenuItemOptionItem
}

type MenuItemOptionItem struct {
	Id             int64
	Option_id      int64
	Option_name    string
	Price_modifier float64
}

type Order struct {
	// Raw fields
	Id       int64
	User_id  int64
	Truck_id int64
	Date     time.Time

	// Go fields
	Items []*OrderItem
}

type OrderItem struct {
	// Raw fields
	Id       int64
	Order_id int64
	Item_id  int64
	Quantity int64
}

type UserData struct {
	// Raw fields
	Id                 int64
	Email              string
	password_hash      string
	password_salt      string
	password_reset_key string

	// Go fields
	orders        []*Order
	Session_token string
}

type Session struct {
	User    *UserData
	Expires time.Time
}

func GetUserById(db *sql.DB, id int64) (*UserData, error) {
	row := db.QueryRow(`SELECT Id, Email, Password_salt, Password_hash, Password_reset_key
        FROM User WHERE Id = ?`, id)
	return readUserLine(row)
}

func GetUserByEmail(db *sql.DB, email string) (*UserData, error) {
	row := db.QueryRow(`SELECT Id, Email, Password_salt, Password_hash, Password_reset_key
        FROM User WHERE Email = ?`, email)
	return readUserLine(row)
}

func readUserLine(row *sql.Row) (*UserData, error) {
	user_data := new(UserData)
	if err := row.Scan(
		&user_data.Id,
		&user_data.Email,
		&user_data.password_salt,
		&user_data.password_hash,
		&user_data.password_reset_key); err != nil {
		return nil, err
	}

	return user_data, nil
}

func GetTrucksNearLocation(db *sql.DB, lat, lon float64, radius float64, open_from, open_til time.Time) ([]*Truck, error) {
	// Speed up the query a bit by doing a rough narrow before calculating
	// all the distances we might not need

	/*
		//TODO: Correct calculations for the bounding box
		rlat_min := math.Max(lat-2, -90.0)
		rlat_max := math.Min(lat+2, 90.0)
		rlon_min := math.Max(lon-2, -180.0)
		rlon_max := math.Min(lon+2, 180.0)
		WHERE Location_lat BETWEEN ? AND ?
		AND   Location_lon BETWEEN ? AND ?
	*/

	rows, err := db.Query(`
		SELECT Id, Name, Location_lat, Location_lon, Open_from, Open_until, Phone, Description,
		( 3959 * acos( cos( radians(?) )
               * cos( radians( Location_lat ) )
               * cos( radians( Location_lon ) - radians(?) )
               + sin( radians(?) )
               * sin( radians( Location_lat ) ) ) ) AS Distance
		FROM Truck
		HAVING Distance < ?
		AND (
			(Open_from <= ? AND Open_until >= ?)
			OR
			(Open_from <= ? AND Open_until >= ?)
			OR
			(Open_from >= ? AND Open_until <= ?)
			OR
			(Open_from <= ? AND Open_until >= ?)
		)
		ORDER BY Distance`,
		lat, lon, lat, radius, open_from, open_from, open_til, open_til,
		open_from, open_til, open_from, open_til,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trucks := make([]*Truck, 0)
	for rows.Next() {
		truck := new(Truck)
		if err := rows.Scan(
			&truck.Id,
			&truck.Name,
			&truck.Location_lat,
			&truck.Location_lon,
			&truck.Open_from,
			&truck.Open_until,
			&truck.Phone,
			&truck.Description,
			&truck.Distance,
		); err != nil {
			return nil, err
		}
		truck.Menus, err = GetMenusForTruck(db, truck.Id)
		if err != nil {
			return nil, err
		}
		if truck.Open_until.After(time.Now()) {
			truck.Open_now = true
		} else {
			truck.Open_now = false
		}
		trucks = append(trucks, truck)
	}
	return trucks, nil
}

func GetMenusForTruck(db *sql.DB, truck_id int64) ([]*Menu, error) {
	rows, err := db.Query(`
		SELECT Id, Truck_id, Name, Description, Flagship
		FROM Menu
		WHERE Truck_id = ?`, truck_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	menus := make([]*Menu, 0)
	for rows.Next() {
		menu := new(Menu)
		if err := rows.Scan(
			&menu.Id,
			&menu.Truck_id,
			&menu.Name,
			&menu.Description,
			&menu.Flagship,
		); err != nil {
			return nil, err
		}
		menu.Items, err = GetItemsForMenu(db, menu)
		if err != nil {
			return nil, err
		}
		menus = append(menus, menu)
	}
	return menus, nil
}

func GetItemsForMenu(db *sql.DB, menu *Menu) ([]*MenuItem, error) {
	rows, err := db.Query(`
		SELECT Id, Menu_id, Name, Price, Description
		FROM MenuItem
		WHERE Menu_id = ?`, menu.Id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuItem, 0)
	for rows.Next() {
		item := new(MenuItem)
		if err := rows.Scan(
			&item.Id,
			&item.Menu_id,
			&item.Name,
			&item.Price,
			&item.Description,
		); err != nil {
			return nil, err
		}
		item.Truck_id = menu.Truck_id
		item.ListOptions, err = GetOptionsForMenuItem(db, item.Id)
		if err != nil {
			return nil, err
		}
		item.ToggleOptions, err = GetToggleOptionsForMenuItem(db, item.Id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func GetToggleOptionsForMenuItem(db *sql.DB, item_id int64) ([]*MenuToggleOption, error) {
	rows, err := db.Query(`
		SELECT Id, Item_id, Name, Price_modifier
		FROM MenuItemToggle
		WHERE Item_id = ?`, item_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuToggleOption, 0)
	for rows.Next() {
		item := new(MenuToggleOption)
		if err := rows.Scan(
			&item.Id,
			&item.Item_id,
			&item.Name,
			&item.Price_modifier,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func GetOptionsForMenuItem(db *sql.DB, item_id int64) ([]*MenuItemOption, error) {
	rows, err := db.Query(`
		SELECT Id, Item_id, Name
		FROM MenuItemOption
		WHERE Item_id = ?`, item_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuItemOption, 0)
	for rows.Next() {
		item := new(MenuItemOption)
		if err := rows.Scan(
			&item.Id,
			&item.Item_id,
			&item.Name,
		); err != nil {
			return nil, err
		}
		item.Values, err = GetOptionValuesForMenuItem(db, item.Id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil

}

func GetMenuItemById(db *sql.DB, item_id int64) (*MenuItem, error) {
	row := db.QueryRow(`
		SELECT Id, Menu_id, Name, Price, Description
		FROM MenuItem
		WHERE Id = ?`, item_id)

	item := new(MenuItem)
	if err := row.Scan(
		&item.Id,
		&item.Menu_id,
		&item.Name,
		&item.Price,
		&item.Description,
	); err != nil {
		return nil, err
	}

	var err error
	item.ListOptions, err = GetOptionsForMenuItem(db, item.Id)
	if err != nil {
		return nil, err
	}

	item.ToggleOptions, err = GetToggleOptionsForMenuItem(db, item.Id)
	if err != nil {
		return nil, err
	}

	return item, nil
}

func GetOptionValuesForMenuItem(db *sql.DB, option_id int64) ([]*MenuItemOptionItem, error) {
	rows, err := db.Query(`
		SELECT Id, Option_id, Option_name, Price_modifier
		FROM MenuItemOptionItem
		WHERE Option_id = ?`, option_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuItemOptionItem, 0)
	for rows.Next() {
		item := new(MenuItemOptionItem)
		if err := rows.Scan(
			&item.Id,
			&item.Option_id,
			&item.Option_name,
			&item.Price_modifier,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil

}
