-- 0015_universes_two_galaxies.sql — widen the test universe to two galaxies.
--
-- Newly registered players spawn at a random slot in galaxy 1..max_galaxies; we
-- run a two-galaxy test server so fresh homeworlds are spread out. Idempotent.
UPDATE universes
SET max_galaxies = 2
WHERE code_name = 'universe_6_niburu'
  AND max_galaxies < 2;
