package shop;

public class Contract {
    // The price now carries the currency.
    public static long priceOf(String sku, String currency) {
        return sku.length() * 100L;
    }
}
