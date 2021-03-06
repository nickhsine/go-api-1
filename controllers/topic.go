package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"twreporter.org/go-api/models"
)

// GetTopics receive HTTP GET method request, and return the topics.
// `query`, `limit`, `offset` and `sort` are the url query params,
// which define the rule we retrieve topics from storage.
func (nc *NewsController) GetTopics(c *gin.Context) (int, gin.H, error) {
	var total int
	var topics []models.Topic = make([]models.Topic, 0)

	err, mq, limit, offset, sort, full := nc.GetQueryParam(c)

	// response empty records if parsing url query param occurs error
	if err != nil {
		return http.StatusOK, gin.H{"status": "ok", "records": topics, "meta": models.MetaOfResponse{
			Total:  total,
			Offset: offset,
			Limit:  limit,
		}}, nil
	}

	if limit == 0 {
		limit = 10
	}

	if sort == "" {
		sort = "-publishedDate"
	}

	if full {
		topics, total, err = nc.Storage.GetFullTopics(mq, limit, offset, sort, nil)
	} else {
		topics, total, err = nc.Storage.GetMetaOfTopics(mq, limit, offset, sort, nil)
	}

	if err != nil {
		return toPostResponse(err)
	}

	// make sure `response.records`
	// would be `[]` rather than  `null`
	if topics == nil {
		topics = make([]models.Topic, 0)
	}

	return http.StatusOK, gin.H{"status": "ok", "records": topics, "meta": models.MetaOfResponse{
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}}, nil
}

// GetATopic receive HTTP GET method request, and return the certain post.
func (nc *NewsController) GetATopic(c *gin.Context) (int, gin.H, error) {
	var topics []models.Topic
	var err error

	slug := c.Param("slug")
	full, _ := strconv.ParseBool(c.Query("full"))

	mq := models.MongoQuery{
		Slug: slug,
	}

	if full {
		topics, _, err = nc.Storage.GetFullTopics(mq, 1, 0, "-publishedDate", nil)
	} else {
		topics, _, err = nc.Storage.GetMetaOfTopics(mq, 1, 0, "-publishedDate", nil)
	}

	if err != nil {
		return toPostResponse(err)
	}

	if len(topics) == 0 {
		return http.StatusNotFound, gin.H{"status": "Record Not Found", "error": "Record Not Found"}, nil
	}

	return http.StatusOK, gin.H{"status": "ok", "record": topics[0]}, nil
}
