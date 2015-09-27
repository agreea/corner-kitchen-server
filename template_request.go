package main

import (
	"log"
	"net/http"
	"strconv"
)

/*
 * Page for accepting a user reservation
 */

type template_request_data struct {
	Guest        *GuestData
	Meal_request *MealRequest
	Meal         *Meal
}

func template_request(helper *TemplateHelpers, r *http.Request) (interface{}, error) {
	request_id_s := r.Form.Get("request")
	request_id, err := strconv.ParseInt(request_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	meal_req, err := GetMealRequestById(helper.db, request_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	req_data := new(template_request_data)
	req_data.Meal_request = meal_req
	req_data.Guest, err = GetGuestById(helper.db, meal_req.Guest_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	req_data.Meal, err = GetMealById(helper.db, meal_req.Meal_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return req_data, nil
}
