# Gumroad API Node.js Client

A Node.js client for the Gumroad API.

## Installation

```bash
npm install gumroad-api
```

## Usage

```javascript
var Gumroad = require("gumroad-api");

var gumroad = new Gumroad({
  token: "<your-gumroad-token>"
});
```

## Products

### Get all products

```javascript
gumroad.getProducts()
  .then(function(products) {
    // products is an array of product objects
  });
```

### Get a product

```javascript
gumroad.getProduct("my-product-id")
  .then(function(product) {
    // product is a product object
  });
```

### Update a product

```javascript
gumroad.updateProduct("my-product-id", {
  // infos
})
.then(function(product) {
  // Product has been updated
});
```

### Toggle a product

```javascript
gumroad.toggleProduct("my-product-id", false)
  .then(function(product) {
    // Product has been disabled/enabled
  });
```

### Delete a product

```javascript
gumroad.deleteProduct("my-product-id")
  .then(function() {
    // Product has been deleted
  });
```

## Sales

### Get all sales

```javascript
gumroad.getSales()
  .then(function(sales) {
    // sales is an array of sale objects
  });
```

### Get a sale

```javascript
gumroad.getSale("my-sale-id")
  .then(function(sale) {
    // sale is a sale object
  });
```

## Offers

### Create an offer

```javascript
gumroad.createOffer("product-id", {
  // infos
})
.then(function(offer) {
  // Offer has been created
});
```

### Update an offer

```javascript
gumroad.updateOffer("product-id", "offer-id", {
  // infos
})
.then(function(offer) {
  // Offer has been updated
});
```

### Delete an offer

```javascript
gumroad.deleteOffer("product-id", "offer-id")
  .then(function() {
    // Offer has been deleted
  });
```

## License keys

### Get all license keys

```javascript
gumroad.getLicenseKeys("product-id")
  .then(function(licenseKeys) {
    // licenseKeys is an array of license key objects
  });
```

### Verify a license key

```javascript
gumroad.verifyLicenseKey("product-id", "license-key")
  .then(function(licenseKey) {
    // licenseKey is a license key object
  });
```

### Create a license key

```javascript
gumroad.createLicenseKey("product-id", {
  // infos
})
.then(function(licenseKey) {
  // License key has been created
});
```

### Update a license key

```javascript
gumroad.updateLicenseKey("product-id", "license-key-id", {
  // infos
})
.then(function(licenseKey) {
  // License key has been updated
});
```

### Delete a license key

```javascript
gumroad.deleteLicenseKey("product-id", "license-key-id")
  .then(function() {
    // License key has been deleted
  });
```

## Subscribers

### Get all subscribers

```javascript
gumroad.getSubscribers()
  .then(function(subscribers) {
    // subscribers is an array of subscriber objects
  });
```

### Get a subscriber

```javascript
gumroad.getSubscriber("my-subscriber-id")
  .then(function(subscriber) {
    // subscriber is a subscriber object
  });
```

## Webhooks

### Create a webhook

```javascript
gumroad.createWebhook("product-id", {
  // infos
})
.then(function(webhook) {
  // Webhook has been created
});
```

### Delete a webhook

```javascript
gumroad.deleteWebhook("product-id", "webhook-id")
  .then(function() {
    // Webhook has been deleted
  });
```