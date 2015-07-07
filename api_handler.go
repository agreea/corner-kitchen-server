package main

import (
	"encoding/json"
	"fmt"
	"github.com/rschlaikjer/go-apache-logformat"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
)

const apache_log_format = `%h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-agent}i"`

type ApiResult struct {
	Success   int
	Return    interface{}
	Error     string
	errorCode int
}

type Servlet interface{}

type ApiHandler struct {
	Servlets  map[string]Servlet
	AccessLog *apachelog.ApacheLog
}

func NewApiHandler(server_config *Config) *ApiHandler {
	h := new(ApiHandler)
	h.SetAccessLog(server_config)
	h.Servlets = make(map[string]Servlet)
	return h
}

func (t *ApiHandler) SetAccessLog(server_config *Config) {
	if !server_config.Arguments.LogToStderr {
		if _, err := os.Stat(server_config.Logging.AccessLogFile); os.IsNotExist(err) {
			log_file, err := os.Create(server_config.Logging.AccessLogFile)
			if err != nil {
				log.Fatal("Log: Create: ", err.Error())
			}
			t.AccessLog = apachelog.NewApacheLog(log_file, apache_log_format)
		} else {
			log_file, err := os.OpenFile(server_config.Logging.AccessLogFile, os.O_APPEND|os.O_RDWR, 0666)
			if err != nil {
				log.Fatal("Log: OpenFile: ", err.Error())
			}
			t.AccessLog = apachelog.NewApacheLog(log_file, apache_log_format)
		}
	} else {
		t.AccessLog = apachelog.NewApacheLog(os.Stderr, apache_log_format)
	}
}

func (t *ApiHandler) AddServlet(endpoint string, handler Servlet) {
	t.Servlets[endpoint] = handler
}

// Deals with incoming HTTP requests. Checks if the appropriate servlet exists,
// and if so gets the servlet method to handle the request.
// If the method is cacheable, also deal with getting/setting cached values.
func (t *ApiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lw := apachelog.NewLoggingWriter(w, r, t.AccessLog)
	defer lw.EmitLog()

	if servlet, servlet_exists := t.Servlets[r.RequestURI]; servlet_exists {
		r.ParseForm()
		method := r.Form.Get("method")

		// Try and get a pointer to the handler method for the request
		// If no handler exists, fail with a Bad Request message.
		method_handler := GetMethodForRequest(servlet, method)
		if method_handler == nil {
			ServeData(w, r,
				APIError(
					fmt.Sprintf("Servlet %s No such method '%s'", r.RequestURI, method),
					400))
			return
		}

		// Perform the method call and unpack the reflect.Value response
		// The prototype for the method returns a single interface{}, which we
		// assert is an ApiResult struct pointer
		args := make([]reflect.Value, 1)
		args[0] = reflect.ValueOf(r)
		response_value := method_handler.Call(args)
		var response_data *ApiResult = nil
		if len(response_value) == 1 {
			response_data = response_value[0].Interface().(*ApiResult)
		}

		if response_data != nil {
			ServeData(w, r, response_data)
		} else {
			ServeData(w, r, APIError("Internal Server Error", 500))
		}
	} else {
		ServeData(w, r, APIError(fmt.Sprintf("No matching servlet for request %s", r.RequestURI), 404))
	}
}

func APIError(error string, errcode int) *ApiResult {
	return &ApiResult{
		Success:   0,
		Error:     error,
		errorCode: errcode,
		Return:    nil,
	}
}

func APISuccess(result interface{}) *ApiResult {
	return &ApiResult{
		Success: 1,
		Return:  result,
	}
}

// JSON encode an ApiResult and write it to the HTTP response.
// Also sets the error code if the ApiResult is an error.
func ServeData(w http.ResponseWriter, r *http.Request, data *ApiResult) {
	data_json, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal server error", 500)
		return
	}
	if data.Success == 0 {
		w.WriteHeader(http.StatusInternalServerError)
	}
	fmt.Fprintf(w, string(data_json))
}

// Write a raw JSON result, e.g. the return of CacheGetRequest
func ServeRawData(w http.ResponseWriter, r *http.Request, result []byte) {
	fmt.Fprintf(w, "%s", result)
}

// To avoid a massive case statement, use reflection to do a lookup of the given
// method on the servlet. MethodByName will return a 'Zero Value' for methods
// that aren't found, which will return false for .IsValid.
// Performing Call() on an unexported method is a runtime violation, uppercasing
// the first letter in the method name before reflection avoids locating
// unexported functions. A little hacky, but it works.
// This method also determines whether a given method can be cached, based again
// on the name that is given to the method. Methods prefixes with Cacheable will
// be reported as such.
//
// For more info, see http://golang.org/pkg/reflect/
func GetMethodForRequest(t interface{}, method string) *reflect.Value {
	if method == "" {
		method = "ServeHTTP"
	}

	upper_method := strings.ToUpper(method)
	exported_method := []byte(method)
	exported_method[0] = upper_method[0]

	// Check if the method is valid
	servlet_value := reflect.ValueOf(t)
	method_handler := servlet_value.MethodByName(string(exported_method))
	if method_handler.IsValid() {
		return &method_handler
	}

	return nil
}
