package pokemons

import (
	"PokeTeamCalculator/database"
	"PokeTeamCalculator/queries"
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetAllPokemons(c *gin.Context) {
	pageString := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageString)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"error":   "bad_page_entry",
			"message": "page parameter needs number",
		})
		return
	}

	data, err := ServiceGetPokemons(page)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  404,
			"error":   "pokemon_not_found",
			"message": "The specified page was not found.",
		})
		return
	}
	c.JSON(http.StatusOK, data)
}

func GetPokemonByID(c *gin.Context) {
	idString := c.Param("id")
	id, err := strconv.Atoi(idString)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"error":   "bad_page_entry",
			"message": "page parameter needs number",
		})
		return
	}
	log.Println("ID: " + idString)
	queryMan := queries.New(database.GetDatabase())
	data, err := queryMan.GetPokemonByID(context.Background(), int32(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  http.StatusNotFound,
			"error":   "pokemon_not_found",
			"message": "the specified pokemon was not found",
		})
		return
	}
	c.JSON(http.StatusOK, data)
}
