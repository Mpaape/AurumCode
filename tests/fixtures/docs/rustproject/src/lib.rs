//! A tiny, synthetic Rust crate used as a deterministic fixture by AUR-427's
//! acceptance test. It hands the native, tool-free documentation extractor
//! (internal/documentation/extractors/rust) real `pub` items and real `///`
//! doc comments to parse, so a green acceptance run proves that actual
//! source was read and rendered instead of an empty pass.
//!
//! It also carries one item this parser deliberately does not recognize
//! (see `record_internal` below): the honest-coverage clause of AUR-427
//! requires that a symbol never shown on the generated page is a symbol the
//! parser genuinely could not extract, not one it silently dropped by
//! accident.

/// One entry in the ledger: an amount in cents and a human-readable memo.
pub struct Entry {
    /// The amount, in cents, so no floating-point rounding ever enters a
    /// balance.
    pub amount_cents: i64,
    /// A short, free-form description of what this entry is for.
    pub memo: String,
}

/// Creates a new ledger entry.
///
/// `amount_cents` may be negative for a debit.
pub fn new_entry(amount_cents: i64, memo: &str) -> Entry {
    Entry {
        amount_cents,
        memo: memo.to_string(),
    }
}

/// The maximum number of entries a single ledger page may hold before this
/// crate asks the caller to start a new page.
pub const MAX_ENTRIES_PER_PAGE: usize = 500;

/// A running total of ledger entries.
pub struct Ledger {
    entries: Vec<Entry>,
}

impl Ledger {
    /// Creates an empty ledger.
    pub fn new() -> Ledger {
        Ledger { entries: Vec::new() }
    }

    /// Adds an entry and returns the ledger's new balance, in cents.
    pub fn add(&mut self, entry: Entry) -> i64 {
        self.entries.push(entry);
        self.balance_cents()
    }

    /// Sums every entry's amount, in cents.
    pub fn balance_cents(&self) -> i64 {
        self.entries.iter().map(|e| e.amount_cents).sum()
    }

    // Not `pub`: never shown on the generated page, and never claimed to be.
    fn entry_count(&self) -> usize {
        self.entries.len()
    }
}

/// An entry kind, used by callers that need to categorize a ledger line.
pub enum EntryKind {
    /// Money coming in.
    Credit,
    /// Money going out.
    Debit,
}

// record_internal is deliberately outside this parser's declared coverage:
// AUR-427's spec states plainly that macro-generated items are invisible to
// a text scanner, by construction. The macro template below spells its
// generated function as `pub fn $name`, which contains no item name a text
// scanner can read (`$name` is not an identifier token until the macro
// expander substitutes it) -- so the generated `record_internal` function
// never appears as literal source text anywhere in this file. The
// honest-coverage proof in tests/unit/AUR-427.go checks exactly that
// absence.
macro_rules! define_record_fn {
    ($name:ident) => {
        pub fn $name() -> bool {
            true
        }
    };
}
define_record_fn!(record_internal);
