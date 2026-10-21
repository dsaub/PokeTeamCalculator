package pokemons

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"PokeTeamCalculator/database"
	"PokeTeamCalculator/queries"

	"github.com/gin-gonic/gin"
)

func GetAllPokemons(c *gin.Context) {
	pageString := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageString)
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"error":   "bad_page_entry",
			"message": "page parameter needs a number greater than 0",
		})
		return
	}

	data, err := ServiceGetPokemons(page)
	if err != nil {
		if errors.Is(err, ErrPaginaInvalida) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  http.StatusBadRequest,
				"error":   "bad_page_entry",
				"message": "page parameter needs a number greater than 0",
			})
			return
		}
		slog.Error("listar_pokemons", "page", page, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  http.StatusInternalServerError,
			"error":   "pokemon_query_failed",
			"message": "The Pokémon list could not be loaded.",
		})
		return
	}

	c.JSON(http.StatusOK, data)
}

func GetPokemonByID(c *gin.Context) {
	idString := c.Param("id")
	id, err := strconv.Atoi(idString)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"error":   "bad_pokemon_id",
			"message": "pokemon id needs a number greater than 0",
		})
		return
	}
	queryMan := queries.New(database.GetDatabase())
	data, err := queryMan.GetPokemonByID(c.Request.Context(), int32(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  http.StatusNotFound,
				"error":   "pokemon_not_found",
				"message": "the specified pokemon was not found",
			})
			return
		}
		slog.Error("pokemon_por_id", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  http.StatusInternalServerError,
			"error":   "pokemon_query_failed",
			"message": "The Pokémon could not be loaded.",
		})
		return
	}
	c.JSON(http.StatusOK, data)
}
