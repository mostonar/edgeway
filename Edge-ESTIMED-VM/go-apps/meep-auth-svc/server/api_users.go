package server

import (
	"encoding/json"
	"net/http"

	users "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-users"
	"github.com/gorilla/mux"
)

type User struct {
	Provider string `json:"provider,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Role     string `json:"role,omitempty"`
	Sboxname string `json:"sboxname,omitempty"`
}

type UserRole struct {
	Role string `json:"role,omitempty"`
}

func checkAdminRole(w http.ResponseWriter, r *http.Request) bool {
	sessionStore := authSvc.sessionMgr.GetSessionStore()
	session, err := sessionStore.Get(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	if session.Role != users.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func GetUsers(w http.ResponseWriter, r *http.Request) {
	if !checkAdminRole(w, r) {
		return
	}

	userMap, err := authSvc.userStore.GetUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var userList []User
	for _, u := range userMap {
		userList = append(userList, User{
			Provider: u.Provider,
			Username: u.Username,
			Role:     u.Role,
			Sboxname: u.Sboxname,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(userList)
}

func UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	if !checkAdminRole(w, r) {
		return
	}

	vars := mux.Vars(r)
	provider := vars["provider"]
	username := vars["username"]

	var userRole UserRole
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&userRole)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = authSvc.userStore.UpdateUser(provider, username, "", userRole.Role, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
}

func DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !checkAdminRole(w, r) {
		return
	}

	vars := mux.Vars(r)
	provider := vars["provider"]
	username := vars["username"]

	err := authSvc.userStore.DeleteUser(provider, username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
}

func CreateUser(w http.ResponseWriter, r *http.Request) {
        if !checkAdminRole(w, r) {
                return
        }

var user User
decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if user.Provider == "" || user.Username == "" || user.Role == "" || user.Password == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if user.Provider != "local" {
		http.Error(w, "Only the local provider is allowed for manual user creation", http.StatusBadRequest)
		return
	}

	err = authSvc.userStore.CreateUser(user.Provider, user.Username, user.Password, user.Role, user.Sboxname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
}
