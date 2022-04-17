package main

import (
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type User struct {
	gorm.Model
	Name    string
	Age     int
	Email   string `gorm:"type:varchar(100);unique_index"`
	Hobbies []Hobby
	Avatar  string
}

type Hobby struct {
	gorm.Model
	UserID uint
	Name   string
}

func ListUsers(db *gorm.DB) ([]*User, error) {
	users := []*User{}
	result := db.Find(&users)
	return users, result.Error
}

func UpdateUserAvatar(db *gorm.DB, userID uint, avatar string) error {
	user := &User{}
	res := db.First(&user, userID)
	if res.Error != nil {
		return res.Error
	}

	user.Avatar = avatar
	res = db.Save(user)
	return res.Error
}

func AddNewUser(db *gorm.DB, name string, age int, email string, hobbies []string) (uint, error) {
	user := User{
		Name:  name,
		Age:   age,
		Email: email,
	}
	result := db.Create(&user)

	if result.Error != nil {
		return 0, result.Error
	}

	var err error
	for _, hobby := range hobbies {
		h := Hobby{
			UserID: user.ID,
			Name:   hobby,
		}
		res := db.Create(&h)
		if res.Error != nil && err == nil {
			err = res.Error
		}
	}

	return user.ID, err
}

func main() {
	// connecting to sqlite database
	db, err := gorm.Open(sqlite.Open("database_gorm.sqlite3"), &gorm.Config{})
	if err != nil {
		panic(fmt.Sprintf("not able to connect to database: %s", err.Error()))
	}

	// Create the required Users tables
	if err := db.AutoMigrate(&User{}); err != nil {
		panic(fmt.Sprintf("not able to create a table %s", err.Error()))
	}

	if err := db.AutoMigrate(&Hobby{}); err != nil {
		panic(fmt.Sprintf("not able to create a table %s", err.Error()))
	}

	//http.Handle("/images/",
	//	http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))

	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		users, err := ListUsers(db)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		indexTmpl := template.Must(template.ParseFiles("templates/index.gohtml"))
		w.Header().Set("Content-Type", "text/html")

		_ = indexTmpl.Execute(w, users)
	})

	http.HandleFunc("/new-user", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/register", http.StatusPermanentRedirect)
			return
		}

		name := r.FormValue("Name")
		ageStr := r.FormValue("Age")
		email := r.FormValue("Email")
		hobbiesStr := r.FormValue("Hobbies")

		age, err := strconv.Atoi(ageStr)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		hobbies := []string{}
		for _, h := range strings.Split(hobbiesStr, ",") {
			hobbies = append(hobbies, strings.TrimSpace(h))
		}

		avatar, avatarFileHeader, err := r.FormFile("Avatar")
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if avatar == nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		userID, err := AddNewUser(db, name, age, email, hobbies)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var ext string
		switch avatarFileHeader.Header.Get("Content-Type") {
		case "image/gif":
			ext = "gif"
		case "image/jpg", "image/jpeg":
			ext = "jpg"
		case "image/png":
			ext = "png"
		default:
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		avatarBytes, err := ioutil.ReadAll(avatar)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("avatar-%d.%s", userID, ext)
		err = os.WriteFile(
			fmt.Sprintf("assets/uploadImage/%s", filename),
			avatarBytes,
			os.ModePerm,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Update avatar
		err = UpdateUserAvatar(db, userID, filename)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/register", http.StatusTemporaryRedirect)
	})

	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err.Error())
	}
}
