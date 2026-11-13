# journeys.feature — wave 2: the shopper journeys the edge composes — browse,
# the cart lifecycle, checkout (open, confirm, in-doubt polling) — through the
# real router over the scripted upstream, with the page's Idempotency-Key
# minted per command.
Feature: The storefront journeys

  Background:
    Given the page holds the state cookie "c=c1&s=s1"

  Scenario: The browse journey relays search and product detail
    Given the upstream answers "GET /products" with status 200 and body {"items":[{"catalogItemId":"p1"}]}
    And the upstream answers "GET /products/p1" with status 200 and body {"catalogItemId":"p1","title":"Steel Kettle"}
    When the page sends "GET /api/storefront/products?q=kettle&priceMin=100"
    Then the edge answers status 200
    And the edge answers with body {"items":[{"catalogItemId":"p1"}]}
    And the last upstream call was "GET /products" carrying the query "priceMin=100&q=kettle"
    When the page sends "GET /api/storefront/products/p1"
    Then the edge answers status 200
    And the edge answers with body {"catalogItemId":"p1","title":"Steel Kettle"}
    And the last upstream call was "GET /products/p1" carrying the query ""

  Scenario: The cart journey relays every command with a minted key
    Given the upstream answers "POST /carts/c1/lines" with status 201 and body {"cartId":"c1"}
    When the page sends "POST /api/storefront/carts/c1/lines" with body {"catalogItemId":"p1","quantity":2} and an Idempotency-Key
    Then the edge answers status 201
    And the last upstream call carries a minted Idempotency-Key that is not "k-bdd"
    Given the upstream answers "PUT /carts/c1/destination" with status 200 and body {"ok":true}
    When the page sends "PUT /api/storefront/carts/c1/destination" with body {"postalCode":"01310"} and an Idempotency-Key
    Then the edge answers status 200
    Given the upstream answers "POST /carts/c1/delivery-quotes" with status 200 and body {"quotes":[]}
    When the page sends "POST /api/storefront/carts/c1/delivery-quotes" with body {} and an Idempotency-Key
    Then the edge answers status 200
    Given the upstream answers "PUT /carts/c1/delivery-selection" with status 200 and body {"selected":true}
    When the page sends "PUT /api/storefront/carts/c1/delivery-selection" with body {"optionId":"d1"} and an Idempotency-Key
    Then the edge answers status 200
    Given the upstream answers "PATCH /carts/c1/lines/p1" with status 200 and body {"quantity":3}
    When the page sends "PATCH /api/storefront/carts/c1/lines/p1" with body {"quantity":3} and an Idempotency-Key
    Then the edge answers status 200
    Given the upstream answers "DELETE /carts/c1/lines/p1" with status 204 and body {}
    When the page sends "DELETE /api/storefront/carts/c1/lines/p1"
    Then the edge answers status 204
    And 6 upstream calls were made

  Scenario: The carts of the cookie's shopper are listed
    Given the upstream answers "GET /carts" with status 200 and body {"items":[{"cartId":"c1"}]}
    When the page sends "GET /api/storefront/carts"
    Then the edge answers status 200
    And the last upstream call was "GET /carts" carrying the query "shopperId=s1"

  Scenario: A checkout opens only for a session the cookie carries
    Given the page holds the state cookie "c=c1"
    When the page sends "POST /api/storefront/checkout" with body {"shipToAddressId":"a1"} and an Idempotency-Key
    Then the edge answers status 422
    And the edge answers the problem "session-required"
    And no upstream was called

  Scenario: An opened checkout keeps its id in the state cookie
    Given the upstream answers "POST /checkout-sessions" with status 201 and body {"sessionId":"cs1"}
    When the page sends "POST /api/storefront/checkout" with body {"shipToAddressId":"a1"} and an Idempotency-Key
    Then the edge answers status 201
    And the edge sets the state cookie "c=c1&k=cs1&s=s1" as HttpOnly, Secure and SameSite=Lax on the storefront path

  Scenario: Raw card data never leaves the edge
    Given the page holds the state cookie "c=c1&s=s1&k=cs1"
    When the page sends "POST /api/storefront/checkout/cs1/confirmations" with body {"cardNumber":"4111111111111111","cvv":"123"} and an Idempotency-Key
    Then the edge answers status 422
    And the edge answers the problem "validation-failed"
    And no upstream was called

  Scenario: A token that is really a card number never leaves the edge
    Given the page holds the state cookie "c=c1&s=s1&k=cs1"
    When the page sends "POST /api/storefront/checkout/cs1/confirmations" with body {"paymentMethodToken":"4111111111111111"} and an Idempotency-Key
    Then the edge answers status 422
    And the edge answers the problem "validation-failed"
    And no upstream was called

  Scenario: A confirmation with the PSP token is relayed
    Given the page holds the state cookie "c=c1&s=s1&k=cs1"
    And the upstream answers "POST /checkout-sessions/cs1/confirmations" with status 202 and body {"status":"IN_DOUBT"}
    When the page sends "POST /api/storefront/checkout/cs1/confirmations" with body {"paymentMethodToken":"tok_visa"} and an Idempotency-Key
    Then the edge answers status 202
    And the edge answers with body {"status":"IN_DOUBT"}

  Scenario: The page polls an in-doubt checkout and the answer passes through
    Given the upstream answers "GET /checkout-sessions/cs1" with status 202 and body {"status":"IN_DOUBT"}
    When the page sends "GET /api/storefront/checkout/cs1"
    Then the edge answers status 202
    And the edge answers with body {"status":"IN_DOUBT"}
    Given the upstream answers "GET /checkout-sessions/cs1" with status 200 and body {"status":"CONFIRMED","orderId":"o1"}
    When the page sends "GET /api/storefront/checkout/cs1"
    Then the edge answers status 200
    And the edge answers with body {"status":"CONFIRMED","orderId":"o1"}

  Scenario: A malformed cart id is refused before any upstream call
    When the page sends "GET /api/storefront/carts/not%20an%20id"
    Then the edge answers status 422
    And the edge answers the problem "validation-failed"
    And no upstream was called
