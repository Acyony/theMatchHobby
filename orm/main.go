package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type User struct {
	gorm.Model
	Name     string
	Age      int
	Email    string `gorm:"type:varchar(100);unique_index"`
	Password string
	Hobbies  []*Hobby
	Avatar   string
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

func GetSingleUser(db *gorm.DB, userID uint) (*User, error) {
	user := &User{}
	res := db.Preload("Hobbies").First(&user, userID)
	return user, res.Error
}

func ListSimilarUsersByHobby(db *gorm.DB, userID uint) ([]*User, error) {
	user := &User{}
	res := db.Preload("Hobbies").First(&user, userID)
	if res.Error != nil {
		return nil, res.Error
	}

	var hobbiesStr []string
	for _, hobby := range user.Hobbies {
		hobbiesStr = append(hobbiesStr, fmt.Sprintf(`'%s'`, hobby.Name))
	}

	users := []*User{}

	res = db.Raw(fmt.Sprintf(`SELECT users.*, h.name as hobby_name FROM Users
         INNER JOIN hobbies h on users.id = h.user_id
         WHERE hobby_name in (%s) AND users.id != ?
         group by users.id`, strings.Join(hobbiesStr, ",")), userID).Scan(&users)

	for _, user := range users {
		var hobbies []*Hobby
		res := db.Raw("SELECT * FROM `hobbies` WHERE `hobbies`.`user_id` = ? AND `hobbies`.`deleted_at` IS NULL", user.ID).Scan(&hobbies)
		if res.Error != nil {
			return nil, res.Error
		}
		user.Hobbies = hobbies
	}

	return users, res.Error
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

func AddNewUser(db *gorm.DB, name string, age int, email string, password string, hobbies []string) (uint, error) {
	var err error

	user := User{
		Name:  name,
		Age:   age,
		Email: email,
	}

	pass, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	user.Password = string(pass)
	result := db.Create(&user)
	if result.Error != nil {
		return 0, result.Error
	}

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
	db, err := gorm.Open(sqlite.Open("database_gorm.sqlite3"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
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

	http.Handle("/assets/",
		http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

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
		password := r.FormValue("Password")
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

		userID, err := AddNewUser(db, name, age, email, password, hobbies)
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
		case "image/webp":
			ext = "webp"
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

		http.Redirect(w, r, fmt.Sprintf("/user-page?userID=%d", userID), http.StatusSeeOther)
	})

	http.HandleFunc("/user-page", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Redirect(w, r, "/register", http.StatusTemporaryRedirect)
			return
		}

		// extract the userID from query string
		userIDStr := r.URL.Query().Get("userID")
		// convert from string to int
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		similar, _ := ListSimilarUsersByHobby(db, uint(userID))
		for _, u := range similar {
			fmt.Println(u.Name)
		}

		// find the user
		user, err := GetSingleUser(db, uint(userID))
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		// render the template
		indexTmpl := template.Must(template.ParseFiles("templates/userPage.gohtml"))
		w.Header().Set("Content-Type", "text/html")

		err = indexTmpl.Execute(w, user)
		if err != nil {
			fmt.Println(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/my-match", func(w http.ResponseWriter, r *http.Request) {
		// extract the userID from query string
		userIDStr := r.URL.Query().Get("userID")
		// convert from string to int
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if r.Method != "GET" {
			http.Redirect(w, r, fmt.Sprintf("/user-page?userID=%d", userID), http.StatusTemporaryRedirect)
			return
		}

		// find the user
		users, err := ListSimilarUsersByHobby(db, uint(userID))
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// render the template
		indexTmpl := template.Must(template.ParseFiles("templates/matchPage.gohtml"))
		w.Header().Set("Content-Type", "text/html")
		err = indexTmpl.Execute(w, map[string]interface{}{
			"UserID": userID,
			"Users":  users,
		})
		if err != nil {
			fmt.Println(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

	})

	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err.Error())
	}
}
