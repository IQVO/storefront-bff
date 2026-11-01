import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

const sidebar: SidebarsConfig = {
  apisidebar: [
    {
      type: "doc",
      id: "api-reference/rest/storefront-bff-api",
    },
    {
      type: "category",
      label: "Catalog",
      link: {
        type: "doc",
        id: "api-reference/rest/catalog",
      },
      items: [
        {
          type: "doc",
          id: "api-reference/rest/search-products",
          label: "Search products",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/list-categories",
          label: "Get the category tree for department navigation",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/get-product",
          label: "Get one product view",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/quote-product-delivery-dates",
          label: "Delivery dates and charges for one unit of a product",
          className: "api-method post",
        },
      ],
    },
    {
      type: "category",
      label: "Carts",
      link: {
        type: "doc",
        id: "api-reference/rest/carts",
      },
      items: [
        {
          type: "doc",
          id: "api-reference/rest/create-cart",
          label: "Create a cart",
          className: "api-method post",
        },
        {
          type: "doc",
          id: "api-reference/rest/list-carts",
          label: "List carts of a shopper",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/get-cart",
          label: "Get a cart",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/add-cart-line",
          label: "Add a line to the cart",
          className: "api-method post",
        },
        {
          type: "doc",
          id: "api-reference/rest/set-cart-line-quantity",
          label: "Change the quantity of a cart line",
          className: "api-method patch",
        },
        {
          type: "doc",
          id: "api-reference/rest/remove-cart-line",
          label: "Remove a cart line",
          className: "api-method delete",
        },
        {
          type: "doc",
          id: "api-reference/rest/set-cart-destination",
          label: "Set the destination postal prefix",
          className: "api-method post",
        },
        {
          type: "doc",
          id: "api-reference/rest/quote-cart-delivery",
          label: "Quote delivery offers for the cart",
          className: "api-method post",
        },
        {
          type: "doc",
          id: "api-reference/rest/select-cart-delivery",
          label: "Select a delivery offer for a shipment group",
          className: "api-method post",
        },
      ],
    },
    {
      type: "category",
      label: "Checkout",
      link: {
        type: "doc",
        id: "api-reference/rest/checkout",
      },
      items: [
        {
          type: "doc",
          id: "api-reference/rest/open-checkout",
          label: "Open a checkout session for the cart",
          className: "api-method post",
        },
        {
          type: "doc",
          id: "api-reference/rest/get-checkout",
          label: "Get a checkout session",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/confirm-checkout",
          label: "Confirm the session with a payment method token",
          className: "api-method post",
        },
      ],
    },
    {
      type: "category",
      label: "Orders",
      link: {
        type: "doc",
        id: "api-reference/rest/orders",
      },
      items: [
        {
          type: "doc",
          id: "api-reference/rest/list-orders",
          label: "List the shopper's orders",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/get-order",
          label: "Get an order with tracking and checkout totals",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/cancel-order",
          label: "Request the cancellation of an order",
          className: "api-method post",
        },
      ],
    },
    {
      type: "category",
      label: "Account",
      link: {
        type: "doc",
        id: "api-reference/rest/account",
      },
      items: [
        {
          type: "doc",
          id: "api-reference/rest/register-shopper",
          label: "Register a shopper",
          className: "api-method post",
        },
        {
          type: "doc",
          id: "api-reference/rest/create-session",
          label: "Start a session by e-mail lookup",
          className: "api-method post",
        },
        {
          type: "doc",
          id: "api-reference/rest/get-me",
          label: "Get the cookie's shopper",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api-reference/rest/add-my-address",
          label: "Save an address for the cookie's shopper",
          className: "api-method post",
        },
      ],
    },
  ],
};

export default sidebar.apisidebar;
