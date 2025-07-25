Feature: Feed Conversion
  As a feed processing system
  I want to convert external feed formats to internal feed models
  So that I can work with a consistent feed representation

  Scenario: Converting a valid gofeed Feed to internal Feed model
    Given I have a gofeed Feed with the following properties:
      | property     | value      |
      | Title        | Test Feed  |
      | Description  | Test Desc  |
      | Link         | http://example.com |
      | FeedType     | rss        |
      | FeedVersion  | 2.0        |
    When I convert it using FromGoFeed
    Then the result should not be nil
    And the converted feed should have the same Title
    And the converted feed should have the same Description
    And the converted feed should have the same Link
    And the converted feed should have the same FeedType
    And the converted feed should have the same FeedVersion

  Scenario: Converting a nil gofeed Feed
    Given I have a nil gofeed Feed
    When I convert it using FromGoFeed
    Then the result should be nil

  Scenario: Converting gofeed Feed with all fields populated
    Given I have a gofeed Feed with complete information:
      | property     | value      |
      | Title        | Test Feed  |
      | Description  | Test Desc  |
      | Link         | http://example.com |
      | FeedType     | rss        |
      | FeedVersion  | 2.0        |
    When I convert it using FromGoFeed
    Then all fields should be correctly copied to the internal Feed model
