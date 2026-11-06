- **Signals reports stay bounded on large shops.** Flow and retention work out their counts in
  ClickHouse instead of carrying every visitor's journey, and each report query runs under a
  60-second and 4 GiB limit by default, which a setting of the same name in `SIGNALS_CH_URL`
  overrides.
- **A funnel with an empty step is refused** instead of running without that step.
