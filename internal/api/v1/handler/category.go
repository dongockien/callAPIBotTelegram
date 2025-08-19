package v1handler

import (
	"log"
	"net/http"

	"dnk.com/hoc-golang/utils"
	"github.com/gin-gonic/gin"
)

type CategoryHandler struct {
}

type GetCategoryByCategoryV1Param struct {
	Category string `uri:"category" binding:"oneof=php python golang"`
}

type PostCategoriesV1Param struct {
	Name   string `form:"name" binding:"required"`
	Status string `form:"status" binding:"required,oneof=1 2"`
}

func NewCategoryHandler() *CategoryHandler {
	return &CategoryHandler{}
}

func (c *CategoryHandler) GetCategoryByCategoryV1(ctx *gin.Context) {
	var params GetCategoryByCategoryV1Param
	if err := ctx.ShouldBindUri(&params); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.HandleValidationErrors(err))

		return
	}

	log.Println("Into GetCategoryByCategoryV1")

	value, exists := ctx.Get("username")
	if !exists {
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": "Missing username"})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Get category by category (V1)",
		"course":  params.Category,
		"username": value,
	})

}

func (c *CategoryHandler) PostCategoriesV1(ctx *gin.Context) {
	var params PostCategoriesV1Param
	if err := ctx.ShouldBind(&params); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.HandleValidationErrors(err))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Post category (V1)",
		"name":  params.Name,
		"status": params.Status,
	})
}
