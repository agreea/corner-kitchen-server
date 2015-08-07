package main

import (
	"net/http"
)

const api_version = 1

type VersionServlet struct {
}

type VersionResponse struct {
	BuildVersion string
	APIVersion   int
}

func NewVersionServlet() *VersionServlet {
	t := new(VersionServlet)
	return t
}

func (t *VersionServlet) ServeHTTP(r *http.Request) *ApiResult {
	return APISuccess(
		VersionResponse{
			build_version,
			api_version,
		},
	)
}
