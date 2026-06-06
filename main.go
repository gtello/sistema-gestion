package main

import (
	"flag"
	"fmt"
	"os"

	"sistema-gestion/cli"
	"sistema-gestion/persistence"
	"sistema-gestion/reading"
	"sistema-gestion/search"
	"sistema-gestion/store"
	"sistema-gestion/web"
)

const dataFile = "books.json"

func main() {
	webMode := flag.Bool("web", false, "Iniciar servidor web (http://localhost:8080)")
	port := flag.String("port", ":8080", "Puerto del servidor web")
	flag.Parse()

	// Se construyen las mismas dependencias para ambos modos.
	// Las interfaces (Repository, Storage, ReadingService, Searcher)
	// permiten que CLI y Web compartan la misma lógica de negocio.
	storage := persistence.NewJSONStorage(dataFile)
	books, err := storage.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error al cargar datos: %v\n", err)
		os.Exit(1)
	}

	repo := store.NewInMemoryRepository(books)
	readingSvc := reading.NewReadingService()
	searcher := &search.InMemorySearcher{}

	if *webMode {
		runWeb(repo, storage, readingSvc, searcher, *port)
	} else {
		// Modo CLI: la consola es la interfaz. Las dependencias se inyectan
		// a través de Run() que las distribuye a cada subcomando.
		cli.RunWithDeps(repo, storage, readingSvc, searcher)
	}
}

// runWeb inicia el servidor HTTP. Centraliza la construcción del servidor
// para que main.go sea una capa fina de orquestación.
func runWeb(repo store.Repository, storage persistence.Storage, readingSvc reading.ReadingService, searcher search.Searcher, port string) {
	srv, err := web.NewServer(repo, storage, readingSvc, searcher)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error al crear servidor web: %v\n", err)
		os.Exit(1)
	}
	if err := srv.Start(port); err != nil {
		fmt.Fprintf(os.Stderr, "Error del servidor: %v\n", err)
		os.Exit(1)
	}
}
