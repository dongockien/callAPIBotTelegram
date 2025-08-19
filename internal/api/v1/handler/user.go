package v1handler

import (
	"log"
	"net/http"

	"dnk.com/hoc-golang/utils"
	"github.com/gin-gonic/gin"
)

type UserHandler struct {
}

type GetUsersByIdV1Param struct {
	ID int `uri:"id" binding:"gt=0"`
}

type GetUserByUuidV1Param struct {
	Uuid string `uri:"uuid" binding:"uuid"`
}

func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

func (u *UserHandler) GetUsersV1(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"message": "List all user (v1)"})

}
func (u *UserHandler) GetUsersByIdV1(ctx *gin.Context) {
	var params GetUsersByIdV1Param
	if err := ctx.ShouldBindUri(&params); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.HandleValidationErrors(err))

		return
	}

	log.Println("Into GetUsersByIdV1")
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Get user by ID (V1)",
		"user_id": params.ID,
	})
}
func (u *UserHandler) GetUsersByUuidV1(ctx *gin.Context) {
	var params GetUserByUuidV1Param
	if err := ctx.ShouldBindUri(&params); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.HandleValidationErrors(err))
	}
	ctx.JSON(http.StatusOK, gin.H{
		"message":   "Get user by UUID (V1)",
		"user_uuid": params.Uuid,
	})
}
func (u *UserHandler) PostUsersV1(ctx *gin.Context) {
	ctx.JSON(http.StatusCreated, gin.H{"message": "Create user (v1)"})
}

func (u *UserHandler) PutUsersV1(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"message": "Update user (v1)"})
}
func (u *UserHandler) DeleteUsersV1(ctx *gin.Context) {
	ctx.JSON(http.StatusNoContent, gin.H{"message": "Delete user (v1)"})
}
