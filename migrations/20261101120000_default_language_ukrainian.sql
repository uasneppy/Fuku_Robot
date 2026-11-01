-- Ukrainian becomes the bot's default language. New users default to 'uk', and
-- rows still on the old English default are moved over. Chats and users that
-- chose another language (es, fr, hi, id, pt, ru) are left alone.
ALTER TABLE "public"."users" ALTER COLUMN "language" SET DEFAULT 'uk'::text;

UPDATE "public"."users" SET "language" = 'uk' WHERE "language" = 'en';

UPDATE "public"."chats" SET "language" = 'uk' WHERE "language" = 'en';
