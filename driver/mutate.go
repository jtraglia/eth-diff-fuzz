package main

import (
	"bytes"
	"math/rand"
	"sort"
)

// Mutation represents the types of mutations that can occur.
type Mutation int

const (
	Replace Mutation = iota
	AddBefore
	AddAfter
	Delete
)

// mutationProbabilities defines the likelihood of each mutation as a value between 0 and 1.
var mutationProbabilities = map[Mutation]float64{
	Replace:   0.01,
	AddBefore: 0.001,
	AddAfter:  0.001,
	Delete:    0.001,
}

// probabilityMap is a sorted map to determine which mutation should occur based on probabilities.
var probabilityMap = buildProbabilityMap()

// buildProbabilityMap constructs a cumulative probability map.
func buildProbabilityMap() []struct {
	CumulativeProbability float64
	Mutation              Mutation
} {
	var cumulativeProbability float64
	probMap := []struct {
		CumulativeProbability float64
		Mutation              Mutation
	}{}

	for mutation, probability := range mutationProbabilities {
		if probability > 0 {
			cumulativeProbability += probability
			probMap = append(probMap, struct {
				CumulativeProbability float64
				Mutation              Mutation
			}{cumulativeProbability, mutation})
		}
	}
	return probMap
}

// findMutation determines the mutation based on a random number.
func findMutation(randomNumber float64) (Mutation, bool) {
	index := sort.Search(len(probabilityMap), func(i int) bool {
		return probabilityMap[i].CumulativeProbability >= randomNumber
	})
	if index < len(probabilityMap) {
		return probabilityMap[index].Mutation, true
	}
	return 0, false
}

// Mutate mutates the input bytes based on predefined probabilities and a random seed.
func Mutate(input []byte, seed int64) []byte {
	random := rand.New(rand.NewSource(seed))
	var out bytes.Buffer

	for _, b := range input {
		randomNumber := random.Float64()
		mutation, ok := findMutation(randomNumber)
		if ok {
			switch mutation {
			case AddBefore:
				before := byte(random.Intn(256))
				out.WriteByte(before)
				out.WriteByte(b)
			case AddAfter:
				after := byte(random.Intn(256))
				out.WriteByte(b)
				out.WriteByte(after)
			case Replace:
				replace := byte(random.Intn(256))
				out.WriteByte(replace)
			case Delete:
				// Skip the current byte (effectively deleting it)
			}
		} else {
			// No mutation, write the original byte
			out.WriteByte(b)
		}
	}
	return out.Bytes()
}