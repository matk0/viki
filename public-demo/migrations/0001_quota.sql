CREATE TABLE quota (
  bucket TEXT NOT NULL,
  day TEXT NOT NULL,
  used INTEGER NOT NULL CHECK (used >= 1),
  PRIMARY KEY (bucket, day)
);
