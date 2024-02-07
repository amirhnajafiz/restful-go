package http

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/amirhnajafiz/ghoster/pkg/enum"
	"github.com/amirhnajafiz/ghoster/pkg/models"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
)

const baseDir = "./files/"

// Upload documents into agent local storage.
func (h HTTP) Upload(ctx echo.Context) error {
	// get title and create uid for document
	title := ctx.FormValue("title")
	uid := uuid.New().String()
	path := baseDir + uid + "."

	// get file from form data
	file, err := ctx.FormFile("project")
	if err != nil {
		h.Logger.Error(err)

		return echo.ErrBadRequest
	}

	// check file extension
	parts := strings.Split(file.Filename, ".")
	if parts[len(parts)-1] != "zip" {
		return echo.ErrBadRequest
	}

	path = path + file.Filename

	// open file
	src, err := file.Open()
	if err != nil {
		h.Logger.Error(err)

		return echo.ErrInternalServerError
	}

	// create local file
	dst, err := os.Create(path)
	if err != nil {
		h.Logger.Error(err)

		return echo.ErrInternalServerError
	}

	// save content
	if _, err = io.Copy(dst, src); err != nil {
		h.Logger.Error(err)

		return echo.ErrInternalServerError
	}

	_ = src.Close()
	_ = dst.Close()

	// create a new document instance
	document := models.Document{
		Title:       title,
		UUID:        uid,
		CreatedAt:   time.Now(),
		Forbidden:   false,
		StoragePath: path,
	}

	// create a new context
	c := context.Background()

	// insert into database
	if _, er := h.DB.Collection(h.Collection).InsertOne(c, document, nil); er != nil {
		h.Logger.Error(er)

		return echo.ErrInternalServerError
	}

	return ctx.NoContent(http.StatusOK)
}

// List returns a list of current uploads.
func (h HTTP) List(ctx echo.Context) error {
	// create context
	c := context.Background()

	// create filter
	filter := bson.D{}

	// query for documents
	cursor, err := h.DB.Collection(h.Collection, nil).Find(c, filter, nil)
	if err != nil {
		h.Logger.Error(err)

		return echo.ErrInternalServerError
	}

	// create a docs list for fetching
	ids := make([]string, 0)

	for cursor.Next(c) {
		var tmp models.Document
		if err := cursor.Decode(&tmp); err != nil {
			h.Logger.Error(err)

			return echo.ErrInternalServerError
		}

		ids = append(ids, tmp.UUID+" "+tmp.Title)
	}

	return ctx.JSON(http.StatusOK, ids)
}

// Use handles the project execution.
func (h HTTP) Use(ctx echo.Context) error {
	// get uid
	uid := ctx.Param("uid")

	// create context
	c := context.Background()

	// create filter
	filter := bson.M{"uuid": uid}

	// fetch the first object
	doc := new(models.Document)

	cursor := h.DB.Collection(h.Collection).FindOne(c, filter, nil)
	if err := cursor.Err(); err != nil {
		h.Logger.Error(err)

		return echo.ErrInternalServerError
	}

	// parse into the docs object
	if err := cursor.Decode(doc); err != nil {
		h.Logger.Error(err)

		return echo.ErrInternalServerError
	}

	// create a new worker
	w, err := h.Agent.NewWorker()
	if err != nil {
		h.Logger.Error(err)

		return echo.ErrInternalServerError
	}

	// get worker stdin and stdout
	stdin := w.GetStdin()
	stdout := w.GetStdout()

	// pass the storage path for starting the process
	stdin <- doc.StoragePath

	// get the result from the process
	result := <-stdout
	msg := result.(string)

	// dismiss the process
	stdin <- enum.CodeDismiss

	// update fields
	update := bson.D{
		{
			"$set",
			bson.D{
				{"forbidden", msg == string(enum.CodeFailure)},
				{"last_execute", time.Now()},
			},
		},
	}

	// update document
	if _, er := h.DB.Collection(h.Collection).UpdateOne(c, filter, update, nil); er != nil {
		h.Logger.Error(er)

		return echo.ErrInternalServerError
	}

	// on failure handler
	if msg == string(enum.CodeFailure) {
		return ctx.NoContent(http.StatusBadRequest)
	}

	return ctx.String(http.StatusOK, msg)
}
