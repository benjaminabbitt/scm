package steps

import (
	"strings"

	"github.com/cucumber/godog"
)

// RegisterFragmentSteps registers steps for fragment operations.
func RegisterFragmentSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^a fragment "([^"]*)" with content:$`, aFragmentWithContent)
	ctx.Step(`^a fragment "([^"]*)" in the project with content:$`, aFragmentInProjectWithContent)
	ctx.Step(`^a fragment "([^"]*)" in home with content:$`, aFragmentInHomeWithContent)
	ctx.Step(`^a prompt "([^"]*)" in the project with content:$`, aPromptInProjectWithContent)
	ctx.Step(`^a prompt "([^"]*)" in home with content:$`, aPromptInHomeWithContent)
	ctx.Step(`^a config file with:$`, aConfigFileWith)
	ctx.Step(`^a home config file with:$`, aHomeConfigFileWith)
	ctx.Step(`^a profile file "([^"]*)" with:$`, aProfileFileWith)
}

func aFragmentWithContent(name string, content *godog.DocString) error {
	return aFragmentInProjectWithContent(name, content)
}

func aFragmentInProjectWithContent(name string, content *godog.DocString) error {
	path := ".scm/context-fragments/" + name + ".yaml"
	return TestEnv.WriteFile(path, content.Content)
}

func aFragmentInHomeWithContent(name string, content *godog.DocString) error {
	path := ".scm/context-fragments/" + name + ".yaml"
	return TestEnv.WriteHomeFile(path, content.Content)
}

func aPromptInProjectWithContent(name string, content *godog.DocString) error {
	path := ".scm/prompts/" + name + ".yaml"
	return TestEnv.WriteFile(path, content.Content)
}

func aPromptInHomeWithContent(name string, content *godog.DocString) error {
	path := ".scm/prompts/" + name + ".yaml"
	return TestEnv.WriteHomeFile(path, content.Content)
}

func aConfigFileWith(content *godog.DocString) error {
	configContent := content.Content
	// Replace mock LM path placeholder if mock LM is configured
	if MockLM != nil {
		configContent = strings.ReplaceAll(configContent, "{{MOCK_LM_PATH}}", MockLM.BinaryPath)
	}
	if err := TestEnv.WriteFile(".scm/config.yaml", configContent); err != nil {
		return err
	}
	// If mock LM is configured, re-apply mock settings while preserving profiles/generators
	if MockLM != nil {
		if err := MockLM.WriteConfig(); err != nil {
			return err
		}
		// Debug: show the final config
		// fmt.Println("DEBUG: Config written to", filepath.Join(MockLM.ProjectDir, ".scm", "config.yaml"))
	}
	return nil
}

func aHomeConfigFileWith(content *godog.DocString) error {
	return TestEnv.WriteHomeFile(".scm/config.yaml", content.Content)
}

func aProfileFileWith(name string, content *godog.DocString) error {
	path := ".scm/profiles/" + name + ".yaml"
	return TestEnv.WriteFile(path, content.Content)
}
