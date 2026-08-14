"""Cart pricing. ``find_item`` may return ``None`` and the caller ignores it."""


def find_item(items, sku):
    for item in items:
        if item["sku"] == sku:
            return item
    return None


def line_total(items, sku, quantity):
    item = find_item(items, sku)
    # The result of find_item is used without checking for None.
    return item["price"] * quantity
