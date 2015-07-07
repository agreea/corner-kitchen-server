package main

import (
	"net/http"
)

type VersionServlet struct {
}

func NewVersionServlet() *VersionServlet {
	t := new(VersionServlet)
	return t
}

func (t *VersionServlet) ServeHTTP(r *http.Request) *ApiResult {
	return APISuccess(build_version)
}
