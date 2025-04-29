package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"golang.org/x/term"

	"knative.dev/func/pkg/docker"
	"knative.dev/func/pkg/docker/creds"
)

func NewPromptForCredentials(in io.Reader, out, errOut io.Writer) func(repository string) (docker.Credentials, error) {
	firstTime := true
	return func(repository string) (docker.Credentials, error) {
		var result docker.Credentials
		if firstTime {
			firstTime = false
			fmt.Fprintf(out, "Please provide credentials for image repository '%s'.\n", repository)
		} else {
			fmt.Fprintf(out, "Incorrect credentials for repository '%s'. Please make sure the repository is correct and try again.\n", repository)
		}

		var qs = []*survey.Question{
			{
				Name: "username",
				Prompt: &survey.Input{
					Message: "Username:",
				},
				Validate: survey.Required,
			},
			{
				Name: "password",
				Prompt: &survey.Password{
					Message: "Password:",
				},
				Validate: survey.Required,
			},
		}

		var (
			fr terminal.FileReader
			ok bool
		)

		isTerm := false
		if fr, ok = in.(terminal.FileReader); ok {
			isTerm = term.IsTerminal(int(fr.Fd()))
		}

		if isTerm {
			err := survey.Ask(qs, &result, survey.WithStdio(fr, out.(terminal.FileWriter), errOut))
			if err != nil {
				return docker.Credentials{}, err
			}
		} else {
			reader := bufio.NewReader(in)

			fmt.Fprintf(out, "Username: ")
			u, err := reader.ReadString('\n')
			if err != nil {
				return docker.Credentials{}, err
			}
			u = strings.Trim(u, "\r\n")

			fmt.Fprintf(out, "Password: ")
			p, err := reader.ReadString('\n')
			if err != nil {
				return docker.Credentials{}, err
			}
			p = strings.Trim(p, "\r\n")

			result = docker.Credentials{Username: u, Password: p}
		}

		return result, nil
	}
}

func NewPromptForCredentialStore() creds.ChooseCredentialHelperCallback {
	return func(availableHelpers []string) (string, error) {
		if len(availableHelpers) < 1 {
			fmt.Fprintf(os.Stderr, `Credentials will not be saved.
If you would like to save your credentials in the future,
you can install docker credential helper https://github.com/docker/docker-credential-helpers.
`)
			return "", nil
		}

		isTerm := term.IsTerminal(int(os.Stdin.Fd()))

		var resp string

		if isTerm {
			err := survey.AskOne(&survey.Select{
				Message: "Choose credentials helper",
				Options: append(availableHelpers, "None"),
			}, &resp, survey.WithValidator(survey.Required))
			if err != nil {
				return "", err
			}
			if resp == "None" {
				fmt.Fprintf(os.Stderr, "No helper selected. Credentials will not be saved.\n")
				return "", nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "Available credential helpers:\n")
			for _, helper := range availableHelpers {
				fmt.Fprintf(os.Stderr, "%s\n", helper)
			}
			fmt.Fprintf(os.Stderr, "Choose credentials helper: ")

			reader := bufio.NewReader(os.Stdin)

			var err error
			resp, err = reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			resp = strings.Trim(resp, "\r\n")
			if resp == "" {
				fmt.Fprintf(os.Stderr, "No helper selected. Credentials will not be saved.\n")
			}
		}

		return resp, nil
	}
}
