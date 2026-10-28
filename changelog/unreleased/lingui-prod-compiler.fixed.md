- **A production dashboard no longer prints raw message syntax.** Any text with a number or a plural
  in it - the table pager's row count and "of N" pages among them - showed up as
  `{totalRows, plural, one {# row} other {# rows}}` instead of the formatted words, in English and in
  installed language packs alike. Development builds were never affected.
