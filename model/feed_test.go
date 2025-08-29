package model

import (
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	"github.com/mmcdole/gofeed"
)

type feedConversionTest struct {
	inputFeed  *gofeed.Feed
	outputFeed *Feed
}

type transportParsingTest struct {
	inputTransport  string
	outputTransport Transport
	parseError      error
}

func (f *feedConversionTest) iHaveAGofeedFeedWithTheFollowingProperties(table *godog.Table) error {
	f.inputFeed = &gofeed.Feed{}

	for _, row := range table.Rows[1:] { // Skip header row
		property := row.Cells[0].Value
		value := row.Cells[1].Value

		switch property {
		case "Title":
			f.inputFeed.Title = value
		case "Description":
			f.inputFeed.Description = value
		case "Link":
			f.inputFeed.Link = value
		case "FeedType":
			f.inputFeed.FeedType = value
		case "FeedVersion":
			f.inputFeed.FeedVersion = value
		default:
			return fmt.Errorf("unknown property: %s", property)
		}
	}

	return nil
}

func (f *feedConversionTest) iHaveANilGofeedFeed() error {
	f.inputFeed = nil
	return nil
}

func (f *feedConversionTest) iHaveAGofeedFeedWithCompleteInformation(table *godog.Table) error {
	return f.iHaveAGofeedFeedWithTheFollowingProperties(table)
}

func (f *feedConversionTest) iConvertItUsingFromGoFeed() error {
	f.outputFeed = FromGoFeed(f.inputFeed)
	return nil
}

func (f *feedConversionTest) theResultShouldNotBeNil() error {
	if f.outputFeed == nil {
		return fmt.Errorf("expected non-nil result, got nil")
	}
	return nil
}

func (f *feedConversionTest) theResultShouldBeNil() error {
	if f.outputFeed != nil {
		return fmt.Errorf("expected nil result, got %+v", f.outputFeed)
	}
	return nil
}

func (f *feedConversionTest) theConvertedFeedShouldHaveTheSameTitle() error {
	if f.outputFeed.Title != f.inputFeed.Title {
		return fmt.Errorf("expected Title %q, got %q", f.inputFeed.Title, f.outputFeed.Title)
	}
	return nil
}

func (f *feedConversionTest) theConvertedFeedShouldHaveTheSameDescription() error {
	if f.outputFeed.Description != f.inputFeed.Description {
		return fmt.Errorf("expected Description %q, got %q", f.inputFeed.Description, f.outputFeed.Description)
	}
	return nil
}

func (f *feedConversionTest) theConvertedFeedShouldHaveTheSameLink() error {
	if f.outputFeed.Link != f.inputFeed.Link {
		return fmt.Errorf("expected Link %q, got %q", f.inputFeed.Link, f.outputFeed.Link)
	}
	return nil
}

func (f *feedConversionTest) theConvertedFeedShouldHaveTheSameFeedType() error {
	if f.outputFeed.FeedType != f.inputFeed.FeedType {
		return fmt.Errorf("expected FeedType %q, got %q", f.inputFeed.FeedType, f.outputFeed.FeedType)
	}
	return nil
}

func (f *feedConversionTest) theConvertedFeedShouldHaveTheSameFeedVersion() error {
	if f.outputFeed.FeedVersion != f.inputFeed.FeedVersion {
		return fmt.Errorf("expected FeedVersion %q, got %q", f.inputFeed.FeedVersion, f.outputFeed.FeedVersion)
	}
	return nil
}

func (f *feedConversionTest) allFieldsShouldBeCorrectlyCopiedToTheInternalFeedModel() error {
	// Verify all fields are copied correctly
	if err := f.theConvertedFeedShouldHaveTheSameTitle(); err != nil {
		return err
	}
	if err := f.theConvertedFeedShouldHaveTheSameDescription(); err != nil {
		return err
	}
	if err := f.theConvertedFeedShouldHaveTheSameLink(); err != nil {
		return err
	}
	if err := f.theConvertedFeedShouldHaveTheSameFeedType(); err != nil {
		return err
	}
	if err := f.theConvertedFeedShouldHaveTheSameFeedVersion(); err != nil {
		return err
	}
	return nil
}

// Transport parsing step definitions
func (t *transportParsingTest) iHaveATransportString(transportStr string) error {
	t.inputTransport = transportStr
	return nil
}

func (t *transportParsingTest) iParseItUsingParseTransport() error {
	result, err := ParseTransport(t.inputTransport)
	t.outputTransport = result
	t.parseError = err
	return nil
}

func (t *transportParsingTest) theResultShouldBeStdioTransport() error {
	if t.outputTransport != StdioTransport {
		return fmt.Errorf("expected StdioTransport, got %v", t.outputTransport)
	}
	return nil
}

func (t *transportParsingTest) theResultShouldBeHTTPWithSSETransport() error {
	if t.outputTransport != HTTPWithSSETransport {
		return fmt.Errorf("expected HTTPWithSSETransport, got %v", t.outputTransport)
	}
	return nil
}

func (t *transportParsingTest) theResultShouldBeUndefinedTransport() error {
	if t.outputTransport != UndefinedTransport {
		return fmt.Errorf("expected UndefinedTransport, got %v", t.outputTransport)
	}
	return nil
}

func (t *transportParsingTest) thereShouldBeNoError() error {
	if t.parseError != nil {
		return fmt.Errorf("expected no error, got %w", t.parseError)
	}
	return nil
}

func (t *transportParsingTest) thereShouldBeAnError() error {
	if t.parseError == nil {
		return fmt.Errorf("expected an error, got nil")
	}
	return nil
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	feedTest := &feedConversionTest{}
	transportTest := &transportParsingTest{}

	// Feed conversion step definitions
	ctx.Step(`^I have a gofeed Feed with the following properties:$`, feedTest.iHaveAGofeedFeedWithTheFollowingProperties)
	ctx.Step(`^I have a nil gofeed Feed$`, feedTest.iHaveANilGofeedFeed)
	ctx.Step(`^I have a gofeed Feed with complete information:$`, feedTest.iHaveAGofeedFeedWithCompleteInformation)
	ctx.Step(`^I convert it using FromGoFeed$`, feedTest.iConvertItUsingFromGoFeed)
	ctx.Step(`^the result should not be nil$`, feedTest.theResultShouldNotBeNil)
	ctx.Step(`^the result should be nil$`, feedTest.theResultShouldBeNil)
	ctx.Step(`^the converted feed should have the same Title$`, feedTest.theConvertedFeedShouldHaveTheSameTitle)
	ctx.Step(`^the converted feed should have the same Description$`, feedTest.theConvertedFeedShouldHaveTheSameDescription)
	ctx.Step(`^the converted feed should have the same Link$`, feedTest.theConvertedFeedShouldHaveTheSameLink)
	ctx.Step(`^the converted feed should have the same FeedType$`, feedTest.theConvertedFeedShouldHaveTheSameFeedType)
	ctx.Step(`^the converted feed should have the same FeedVersion$`, feedTest.theConvertedFeedShouldHaveTheSameFeedVersion)
	ctx.Step(`^all fields should be correctly copied to the internal Feed model$`, feedTest.allFieldsShouldBeCorrectlyCopiedToTheInternalFeedModel)

	// Transport parsing step definitions
	ctx.Step(`^I have a transport string "([^"]*)"$`, transportTest.iHaveATransportString)
	ctx.Step(`^I parse it using ParseTransport$`, transportTest.iParseItUsingParseTransport)
	ctx.Step(`^the result should be StdioTransport$`, transportTest.theResultShouldBeStdioTransport)
	ctx.Step(`^the result should be HttpWithSSETransport$`, transportTest.theResultShouldBeHTTPWithSSETransport)
	ctx.Step(`^the result should be UndefinedTransport$`, transportTest.theResultShouldBeUndefinedTransport)
	ctx.Step(`^there should be no error$`, transportTest.thereShouldBeNoError)
	ctx.Step(`^there should be an error$`, transportTest.thereShouldBeAnError)
	ctx.Step(`^the error state should be false$`, transportTest.thereShouldBeNoError)
	ctx.Step(`^the error state should be true$`, transportTest.thereShouldBeAnError)
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}
