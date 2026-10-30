package main

func main() {
	if err := godontenv.Load(); err != nil {
		panic(err)
}