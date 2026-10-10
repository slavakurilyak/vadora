# Fixtures

This directory holds the platform fixtures and the descriptions of the labeled test stores. It holds no credential, no key, and no token.

1. A platform fixture is a clearly labeled test store that exercises a connector without a real merchant. Its results never count as a live customer or as revenue.
2. `shopify/development-store.json` describes the transfer disabled Shopify development store that the test stage uses.
3. `shopify/products.json` is the sample catalog of that store. Every title starts with TEST so that a person never mistakes it for a real listing.
