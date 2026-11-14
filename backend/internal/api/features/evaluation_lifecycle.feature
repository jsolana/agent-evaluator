Feature: Multi-turn evaluation lifecycle
  As the person evaluating a conversational agent
  I want to run multi-turn scenarios, review every turn and compare runs
  So that I can see where a conversation breaks and whether a change helped

  The agent is a deterministic fake: it echoes "answer <n>: <question>" with a
  search call, except for questions about the "cuota", where it answers
  "La cuota sería de S/ 1,500 al mes" calling search and getSource.

  Background:
    Given the evaluator is running with a fake agent

  Scenario: An invalid suite is rejected with every problem located
    When I import the suite:
      """yaml
      name: Broken
      scenarios:
        - key: only-one-turn
          turns:
            - question: "¿Qué es el crédito MiVivienda?"
              criteria: [{type: llm_jugde, expected: "x"}]
      """
    Then the import is rejected with the errors:
      | scenarios[0].turns: has 1 turns, minimum is 3                  |
      | scenarios[0].turns[0].criteria[0]: unknown type "llm_jugde"    |
    And no suite exists

  Scenario: Evaluate, review, annotate, fix a rubric and compare
    # Import and inspect
    When I import the suite:
      """yaml
      name: Hipotecas
      scenarios:
        - key: mortgage
          name: "mortgage | follow-up"
          turns:
            - question: "¿Qué es el crédito MiVivienda?"
              criteria: [{type: regex, pattern: "answer 1"}, {type: tools_required, tools: [search]}]
            - question: "¿Y cuánto sería la cuota?"
              criteria: [{type: regex, pattern: "S/ 1,200"}]
            - question: "Resume lo anterior"
              criteria: [{type: regex, pattern: "answer 3"}]
        - key: stable
          turns:
            - question: "q1"
              criteria: [{type: regex, pattern: "answer 1"}]
            - question: "q2"
              criteria: [{type: regex, pattern: "answer 2"}]
            - question: "q3"
              criteria: [{type: regex, pattern: "answer 3"}]
      """
    Then suite "Hipotecas" version 1 is created with 2 scenarios
    And scenario "mortgage" of the suite has 3 turns
    When I import the same suite again
    Then the existing version 1 is returned

    # Evaluate and review every turn
    When I evaluate the latest version of "Hipotecas" as "baseline"
    Then evaluation "baseline" completes with 1 passed and 1 failed scenarios
    And in "baseline" scenario "mortgage" fails first at turn 2
    And in "baseline" the turns of scenario "mortgage" are:
      | turn | passed | answer                            | tools                         |
      | 1    | yes    | answer 1: ¿Qué es el crédito MiVivienda? | search               |
      | 2    | no     | La cuota sería de S/ 1,500 al mes | search, getSource |
      | 3    | yes    | answer 3: Resume lo anterior      | search                     |
    And in "baseline" turn 2 of "mortgage" fails criterion "regex" explaining "pattern not found: S/ 1,200"
    And in "baseline" the trace of scenario "mortgage" is:
      | turn | calls                         | tool ms | turn ms |
      | 1    | search                     | 120     | 0       |
      | 2    | search, getSource | 300     | 0       |
      | 3    | search                     | 120     | 0       |

    # Annotate the failure
    When I annotate turn 2 of "mortgage" in "baseline" as "rubric_too_strict":
      """
      El corpus dice S/ 1,500: la rúbrica es incorrecta
      """
    Then turn 2 of "mortgage" in "baseline" has 1 annotation tagged "rubric_too_strict"

    # Ask the LLM for a second opinion (deterministic fake analyzer)
    When I analyze turn 2 of "mortgage" in "baseline"
    Then the analysis says "false_positive" because of the "rubric"
    And turn 2 of "mortgage" in "baseline" shows that latest analysis

    # Fix the rubric in a new version and re-run
    When I create a new version of "Hipotecas" replacing "S/ 1,200" with "S/ 1,500"
    Then suite "Hipotecas" version 2 is created with 2 scenarios
    When I evaluate the latest version of "Hipotecas" as "rubric fix"
    Then evaluation "rubric fix" completes with 2 passed and 0 failed scenarios
    And suite "Hipotecas" lists 2 evaluations across its versions

    # Compare at every level
    When I compare "rubric fix" with "baseline"
    Then the scenario pass rate changes by 0.5
    And the scenarios compare as:
      | scenario | change    | outcome     |
      | mortgage | changed   | improvement |
      | stable   | unchanged | stable_pass |
    And the turns of scenario "mortgage" compare as:
      | turn | change    | outcome     |
      | 1    | unchanged | stable_pass |
      | 2    | changed   | improvement |
      | 3    | unchanged | stable_pass |
    And turn 2 of "mortgage" shows the baseline annotation and its tool calls next to the passing target

  Scenario: Analyze every failed turn of an evaluation in the background
    When I import the suite:
      """yaml
      name: Batch
      scenarios:
        - key: a
          turns:
            - question: "q1"
              criteria: [{type: regex, pattern: "answer 1"}]
            - question: "q2"
              criteria: [{type: regex, pattern: "nope"}]
            - question: "q3"
              criteria: [{type: regex, pattern: "nope"}]
      """
    And I evaluate the latest version of "Batch" as "batch"
    Then evaluation "batch" completes with 0 passed and 1 failed scenarios
    When I analyze the failed turns of "batch"
    Then 2 turns are queued for analysis
    And turns 2, 3 of "a" in "batch" get the analysis "real_failure" while turn 1 has none
