package server

import (
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

func Logger(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		inner.ServeHTTP(w, r)

		log.Println(
			r.Method, " ",
			r.RequestURI, " ",
			name, " ",
			time.Since(start),
		)
	})
}

type HttpRoute struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type HttpRoutes []HttpRoute

// CleanPath normalizes the request path (e.g. "//meep_ipe/v1/..." -> "/meep_ipe/v1/...")
func CleanPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "" {
			p = "/"
		}
		cleaned := path.Clean(p)
		// Keep the trailing slash, same as gorilla/mux cleanPath
		if strings.HasSuffix(p, "/") && cleaned != "/" {
			cleaned += "/"
		}
		if cleaned != r.URL.Path {
			log.Printf("CleanPath: rewriting '%s' to '%s'", r.URL.Path, cleaned)
			r.URL.Path = cleaned
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}

func NewRouter(config map[string]interface{}) *mux.Router {
	// No StrictSlash: its trailing slash redirect turns a POST into a GET as well
	router := mux.NewRouter()

	for _, route := range getRoutes(config) {
		fullPath := path.Join(route.Pattern)
		if strings.HasSuffix(route.Pattern, "/") && !strings.HasSuffix(fullPath, "/") {
			fullPath += "/"
		}

		router.
			Methods(route.Method).
			Path(fullPath).
			Name(route.Name).
			Handler(Logger(route.HandlerFunc, route.Name))

		log.Printf("Registered route: %s %s -> %s", route.Method, fullPath, route.Name)
	}

	// Static file server under the base path
	apiPrefix := path.Join("/meep_ipe/v1")
	handler := http.StripPrefix(apiPrefix, http.FileServer(http.Dir("./api/")))
	router.
		PathPrefix(apiPrefix).
		Name("Api").
		Handler(handler)

	// Catch-all for unmatched paths — logs the request so you can see
	// exactly what path/method hit an endpoint that doesn't exist.
	router.NotFoundHandler = http.HandlerFunc(NotFoundHandler)

	// Catch-all for paths that exist but wrong HTTP method
	router.MethodNotAllowedHandler = http.HandlerFunc(MethodNotAllowedHandler)
	return router
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[404] No route matched — Method: %s, Path: %s, RemoteAddr: %s, Host: %s",
		r.Method, r.URL.Path, r.RemoteAddr, r.Host)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, `{"error":"not found","path":"%s","method":"%s"}`, r.URL.Path, r.Method)
}

// MethodNotAllowedHandler logs requests that matched a path but used
// the wrong HTTP method.
func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[405] Method not allowed — Method: %s, Path: %s, RemoteAddr: %s",
		r.Method, r.URL.Path, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	fmt.Fprintf(w, `{"error":"method not allowed","path":"%s","method":"%s"}`, r.URL.Path, r.Method)
}

func Index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello World from MEC IPE!")
}

func getRoutes(config map[string]interface{}) HttpRoutes {
	return HttpRoutes{
		HttpRoute{
			"Index",
			"GET",
			"/mec_ipe/v1/",
			Index,
		},
		HttpRoute{
			"MecIotPltfNotificationHandler",
			"POST",
			"/meep_ipe/v1/mep_iot_notify/meep_ipe",
			MecIotPltfNotificationHandler,
		},
		HttpRoute{
			"ServiceAvailabilityNotificationHandler",
			"POST",
			"/meep_ipe/v1/service_availability_notification",
			ServiceAvailabilityNotificationHandler,
		},
		HttpRoute{
			"ServiceTerminationNotificationHandler",
			"POST",
			"/meep_ipe/v1/service_termination_notification",
			ServiceTerminationNotificationHandler,
		},
		HttpRoute{
			"MecIotPltfNotificationHandlerGET",
			"GET",
			"/meep_ipe/v1/mep_iot_notify/meep_ipe",
			MecIotPltfNotificationHandlerGET,
		},
		HttpRoute{
			"SensorRegisterPostHandler",
			"POST",
			"/meep_ipe/v1/sensor_ae",
			SensorRegisterPostHandler,
		},

		// New endpoint — AE name segment comes from config
		HttpRoute{
			"MecIotDeviceNotificationHandlerAe",
			"POST",
			fmt.Sprintf("/meep_ipe/v1/mep_iot_notify/meep_ipe/%s", config["AE-ID"]),
			DeviceNotificationHandlerAe,
		},
		// Endpoint to recieve notifications from the oneM2M platform for Created Node
		HttpRoute{
			"OneM2MNodeCreationNotificationHandler",
			"POST",
			"/meep_ipe/v1/oneM2M_node_creation",
			NodeCreationNotificationHandler,
		},
	}
}
