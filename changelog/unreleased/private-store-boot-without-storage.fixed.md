- **Development starts when the private store isn't up.** With `PRIVATE_S3_BUCKET` set and no
  Garage running, core refused to boot in every environment, CI's browser suite included. Now
  only production refuses; development logs a warning and only what needs the private store fails.
