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
		c.Error(err)
	}

	data, err := ServiceGetPokemons(page)
	if err != nil {
		c.Error(err)
	}
	c.JSON(http.StatusOK, data)
}

func GetPokemonByID(c *gin.Context) {
	idString := c.Param("id")
	id, err := strconv.Atoi(idString)
	if err != nil {
		c.Error(err)
		return
	}
	log.Println("ID: " + idString)
	queryMan := queries.New(database.GetDatabase())
	data, err := queryMan.GetPokemonByID(context.Background(), int32(id))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, data)
}
