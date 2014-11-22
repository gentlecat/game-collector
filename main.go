package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"code.google.com/p/goconf/conf"
	"github.com/gorilla/mux"
	"github.com/tsukanov/beaten-games/data"
	"github.com/tsukanov/go-giantbomb"
)

func main() {
	fmt.Println("Loading configuration...")
	config, err := conf.ReadConfigFile("config.txt")
	if err != nil {
		log.Fatal("Failed to load config file! ", err)
	}
	giantbomb.Key, err = config.GetString("default", "giant_bomb_api_key")
	if err != nil {
		log.Fatal("Failed to get Giant Bomb API key from config file!", err)
	}

	fmt.Println("Starting server on localhost:8080...")
	err = http.ListenAndServe(":8080", makeRouter())
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func makeRouter() *mux.Router {
	r := mux.NewRouter().StrictSlash(true)

	// Regular pages
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/games/{id:[0-9]+}", gameHandler)
	r.HandleFunc("/games/add", addHandler).Methods("GET", "POST")
	r.HandleFunc("/games/quick-add", quickAddHandler).Methods("POST")
	r.HandleFunc("/games/delete", deleteHandler).Methods("POST")

	r.HandleFunc("/suggest/games", suggestGamesHandler)

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/",
		http.FileServer(http.Dir("static"))))

	return r
}

// executeTemplates is a custom tempate executor that uses our template
// structure. Should be used when rendering templates based on "base.html"
// template.
func executeTemplates(wr io.Writer, data interface{}, filenames ...string) error {
	filenames = append(filenames, "templates/base.html")
	t, err := template.ParseFiles(filenames...)
	if err != nil {
		return err
	}
	return t.ExecuteTemplate(wr, "base", data)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	games, err := data.GetAllGames()
	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to get games.", http.StatusInternalServerError)
		return
	}
	err = executeTemplates(w, struct{ Games []data.Game }{games},
		"templates/index.html")
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
}

func gameHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Failed to parse game ID.", http.StatusInternalServerError)
		return
	}

	// TODO: Implement game lookup

	err = executeTemplates(w, struct{ ID int }{id}, "templates/game.html")
	if err != nil {
		http.Error(w, "Failed to execute template.", http.StatusInternalServerError)
		return
	}
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		err := executeTemplates(w, nil, "templates/add.html")
		if err != nil {
			http.Error(w, "Failed to execute template.", http.StatusInternalServerError)
			return
		}

	} else { // POST (new game is sumbitted)
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Failed to parse submitted form.", http.StatusInternalServerError)
			return
		}
		vals := r.Form
		var game data.Game
		game.Name = vals.Get("name")
		game.Note = sql.NullString{
			String: vals.Get("note"),
			Valid:  true,
		}
		if len(r.Form["beaten_on"][0]) > 0 {
			parsed, err := time.Parse("2006-01-02", r.Form["beaten_on"][0])
			if err != nil {
				http.Error(w, "Failed to parse date.", http.StatusBadRequest)
				return
			}
			game.BeatenOn = data.NullTime{
				Time:  parsed,
				Valid: true,
			}
		} else {
			game.BeatenOn = data.NullTime{
				Valid: true,
			}
		}

		err = data.AddGame(game)
		if err != nil {
			http.Error(w, "Failed to add a game.", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func quickAddHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse submitted form.", http.StatusInternalServerError)
		return
	}
	vals := r.Form
	var game data.Game
	game.Name = vals.Get("name")
	game.Note = sql.NullString{
		Valid: false,
	}
	game.BeatenOn = data.NullTime{
		Time:  time.Now(),
		Valid: true,
	}

	err = data.AddGame(game)
	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to add a game.", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse submitted form.", http.StatusInternalServerError)
		return
	}
	vals := r.Form
	name := vals.Get("name")
	rowsAffected, err := data.DeleteGame(vals.Get("name"))
	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to delete a game.", http.StatusInternalServerError)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "Can't find this game.", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func suggestGamesHandler(w http.ResponseWriter, r *http.Request) {
	qvals := r.URL.Query()
	query, ok := qvals["q"]
	if !ok {
		http.Error(w, "Query is empty.", http.StatusBadRequest)
		return
	}

	giantbomb.FieldList = []string{"id", "name", "platforms"}
	resp, err := giantbomb.Search(query[0], 10, 1, []string{giantbomb.ResourceTypeGame})
	if err != nil {
		http.Error(w, "Search failed.", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	b, err := json.Marshal(resp.Results)
	if err != nil {
		http.Error(w, "Internal error.", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
