DELETE FROM "guest"."resource"
WHERE "booking_id" = {{booking_id}}
RETURNING "id"