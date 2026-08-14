# AUR-014 spec fixtures

The executable specification for this card is the local catalog and its
positive/negative fixture pairs under `standards/code-review/`. This directory
documents the fixed test vectors without introducing another runtime source of
truth:

- `AC-001` requires the nine controls in the AUR-014 card, one catalog row per
  control, one positive case, and one negative case;
- all 18 case fixtures are local regular files, each names its rule ID, and no
  fixture path is shared by another case;
- source metadata must be current, versioned, dated, HTTPS, and allowlisted;
- the acceptance program self-tests `MUT-001` source removal and
  `MUT-002` source substitution, then restores the synthetic fixtures before
  validating the repository catalog; and
- an empty or malformed acceptance environment is inconclusive, never a green
  result.

The catalog README carries the human-readable `CR -> source/version ->
positive/negative -> card/AC/mutation` crosswalk. The acceptance program stays
offline and does not read `.board/cards` at runtime.
