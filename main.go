package main

import (
	codehandlers "PokeTeamCalculator/endpoints/code_handlers"
	"PokeTeamCalculator/endpoints/pokemons"
	"fmt"
	"log"
	"os"

	"PokeTeamCalculator/database"

	"github.com/gin-gonic/gin"
	_ "github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("Hello World")
	database.Initialize()

	router := gin.Default()
	router.GET("/api/v1/pokemon", pokemons.GetAllPokemons)
	router.GET("/api/v1/pokemon/:id", pokemons.GetPokemonByID)

	router.NoRoute(codehandlers.ErrorNotFound)
	err := router.Run(":" + os.Getenv("PORT"))
	if err != nil {
		log.Fatalf("Error %v", err)
	}
}
