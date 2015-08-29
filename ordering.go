package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
)

func CalculateOrderCost(db *sql.DB, order *Order) (float64, error) {
	var total float64 = 0
	for _, orderitem := range order.Items {
		cost, err := CalculateOrderItemCost(db, orderitem)
		if err != nil {
			return 0, err
		}
		total += cost
	}
	return total, nil
}

func CalculateOrderItemCost(db *sql.DB, orderitem *OrderItem) (float64, error) {
	var cost float64 = 0
	log.Println(fmt.Sprintf("GetMenuItemById: %d", orderitem.Item_id))
	baseitem, err := GetMenuItemById(db, orderitem.Item_id)
	if err != nil {
		return 0, err
	}
	cost = baseitem.Price

	for _, toggledOption := range orderitem.ToggleOptions {
		log.Println(fmt.Sprintf("GetToggleOptionCost: %d", toggledOption))
		cost_modifier, err := GetToggleOptionCost(db, toggledOption)
		if err != nil {
			return 0, err
		}
		cost += cost_modifier
	}

	for _, listOption := range orderitem.ListOptionValues {
		log.Println(fmt.Sprintf("GetListOptionCost: %d", listOption))
		cost_modifier, err := GetListOptionCost(db, listOption)
		if err != nil {
			return 0, err
		}
		cost += cost_modifier
	}

	cost = cost * float64(orderitem.Quantity)

	return cost, nil
}

func GetToggleOptionCost(db *sql.DB, toggledValue int64) (float64, error) {
	log.Println(fmt.Sprintf("GetToggleOptionById: %d", toggledValue))
	toggleOption, err := GetToggleOptionById(db, toggledValue)
	if err != nil {
		return 0, err
	}
	return toggleOption.Price_modifier, nil
}

func GetListOptionCost(db *sql.DB, selectedValue int64) (float64, error) {
	log.Println(fmt.Sprintf("GetListOptionValueById: %d", selectedValue))
	listOption, err := GetListOptionValueById(db, selectedValue)
	if err != nil {
		return 0, err
	}
	return listOption.Price_modifier, nil
}
