package shop;

public class Caller {
    public long total(String sku) {
        // priceOf used to take only the sku
        return Contract.priceOf(sku);
    }
}
