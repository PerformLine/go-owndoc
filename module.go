package main

type Metadata struct {
	Title            string
	GeneratorVersion string
	URL              string
}

type Module struct {
	Metadata Metadata
	Package  *Package
}
