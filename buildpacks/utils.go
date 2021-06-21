package buildpacks

import (
	"sort"
	"strings"
)

//Runtimes returns the list of supported runtimes
//as comma seperated strings, sorted alphabetically
func Runtimes() string {
	runtimes := RuntimesList()

	//make it more grammatical :)
	s := runtimes[:len(runtimes)-1]
	str := strings.Join(s, ", ")
	str = str + " and " + runtimes[len(runtimes)-1]
	return str
}

//RuntimesList returns the list of supported runtimes
//as an array of strings, sorted alphabetically
func RuntimesList() []string {
	rb := RuntimeToBuildpack
	runtimes := make([]string, 0, len(rb))
	for k := range rb {
		runtimes = append(runtimes, k)
	}
	sort.Strings(runtimes)

	return runtimes
}
