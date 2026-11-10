Feature: The storefront edge relays and guards upstream calls
  The BFF composes the upstream services for the storefront page. It keeps the
  shopper's cart in a state cookie, relays upstream answers unchanged, mints
  its own upstream Idempotency-Key from the page's key, and turns an upstream
  that does not answer into a problem+json answer of its own. It stores nothing.

  Scenario: Creating a cart stores the cart id in the state cookie
    Given the upstream answers "POST /carts" with status 201 and body {"cartId":"c-42"}
    When the page sends "POST /carts" with body {} and an Idempotency-Key
    Then the edge answers status 201
    And the edge sets the state cookie "c=c-42" as HttpOnly, Secure and SameSite=Lax on the storefront path

  Scenario: The category tree is relayed unchanged and the page's query stays at the edge
    Given the upstream answers "GET /categories" with status 200 and body {"version":2,"categories":[{"categoryId":"home","name":"Home","path":"Home","purchasableCount":3}]}
    When the page sends "GET /api/storefront/categories?evil=1"
    Then the edge answers status 200
    And the edge answers with content type "application/json"
    And the edge answers with body {"version":2,"categories":[{"categoryId":"home","name":"Home","path":"Home","purchasableCount":3}]}
    And the last upstream call was "GET /categories" with no query and no Idempotency-Key

  Scenario: An upstream problem passes through unchanged
    Given the upstream answers "GET /carts/c1" with the problem status 404 and body {"type":"/problems/not-found"}
    When the page sends "GET /carts/c1"
    Then the edge answers status 404
    And the edge answers with content type "application/problem+json"
    And the edge answers with body {"type":"/problems/not-found"}

  Scenario Outline: An upstream that does not answer becomes an edge problem
    Given the upstream fails with <failure>
    When the page sends "GET /products/p1"
    Then the edge answers status <status>
    And the edge answers the problem "<slug>"

    Examples:
      | failure           | status | slug                 |
      | a timeout         | 504    | gateway-timeout      |
      | a transport error | 503    | upstream-unavailable |

  Scenario: A command without an Idempotency-Key never reaches an upstream
    When the page sends "POST /carts" with body {} and no Idempotency-Key
    Then the edge answers status 400
    And the edge answers the problem "idempotency-key-required"
    And no upstream was called

  Scenario: The upstream key is minted from the page's key, never forwarded raw
    When the page sends "POST /carts" with body {} and the Idempotency-Key "k-page-1"
    Then the edge answers status 200
    And the last upstream call carries a minted Idempotency-Key that is not "k-page-1"

  Scenario: A replayed upstream answer is flagged to the page
    Given the page holds the state cookie "c=c1&k=k1&s=s1"
    And the upstream answers "POST /carts/c1/lines" from its idempotency store with body {"cartId":"c1"}
    When the page sends "POST /carts/c1/lines" with body {"catalogItemId":"i1","quantity":1} and an Idempotency-Key
    Then the edge answers status 200
    And the edge answers the header "Idempotency-Replayed" as "true"
    And the edge answers with body {"cartId":"c1"}
