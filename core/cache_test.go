package core

import (
	"testing"
)

func TestCache(t *testing.T) {
	redisClient := NewTestRedisClient()
	c := NewCache(redisClient)

	s := &Recipe{
		Name: "foobar",
	}
	err := c.Set(s, "test", "recipe", "foobar")
	if err != nil {
		t.Fatal(err)
	}

	var g *Recipe
	err = c.Get(&g, "should_not_exists")
	if err != nil {
		t.Fatal(err)
	}
	if g != nil {
		t.Fatalf("want %+v, got %+v", nil, g)
	}

	err = c.Get(&g, "test", "recipe", "foobar")
	if err != nil {
		t.Fatal(err)
	}
	output := g.Name
	expected := "foobar"
	if output != expected {
		t.Fatalf("want %+v, got %+v", expected, output)
	}
}
