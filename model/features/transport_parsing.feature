Feature: Transport Parsing
  As a feed processing system
  I want to parse transport strings into Transport types
  So that I can configure the appropriate communication transport

  Scenario: Parsing valid transport strings
    Given I have a transport string "stdio"
    When I parse it using ParseTransport
    Then the result should be StdioTransport
    And there should be no error

  Scenario: Parsing HTTP with SSE transport
    Given I have a transport string "http-with-sse"
    When I parse it using ParseTransport
    Then the result should be HttpWithSSETransport
    And there should be no error

  Scenario: Parsing invalid transport string
    Given I have a transport string "invalid"
    When I parse it using ParseTransport
    Then the result should be UndefinedTransport
    And there should be an error

  Scenario: Parsing empty transport string
    Given I have a transport string ""
    When I parse it using ParseTransport
    Then the result should be UndefinedTransport
    And there should be an error

  Scenario Outline: Parsing various transport strings
    Given I have a transport string "<input>"
    When I parse it using ParseTransport
    Then the result should be <expected_transport>
    And the error state should be <has_error>

    Examples:
      | input         | expected_transport    | has_error |
      | stdio         | StdioTransport        | false     |
      | http-with-sse | HttpWithSSETransport  | false     |
      | invalid       | UndefinedTransport    | true      |
      |               | UndefinedTransport    | true      |
      | STDIO         | UndefinedTransport    | true      |
      | http          | UndefinedTransport    | true      |
