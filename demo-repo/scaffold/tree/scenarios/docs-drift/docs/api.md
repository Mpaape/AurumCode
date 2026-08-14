# Cart API

## `line_total(items, sku, quantity)`

Returns the price of one cart line.

Documented behaviour: returns `0` when the SKU is unknown.

Actual behaviour in `scenarios/null-deref/src/cart.py`: the lookup returns
`None` and the caller dereferences it, so an unknown SKU raises instead of
returning `0`. The document and the code disagree.
