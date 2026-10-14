-- SsangYong was renamed KG Mobility; TSB lists both makes (SSANGYONG and
-- KGMOBILITY, with Korando and Musso under each). Show them as one display
-- brand, "KG Mobility (SsangYong)". Display brands an operator already
-- renamed are left alone. No-op on databases without catalog data.
DO $$
DECLARE
    kgm_id    BIGINT;
    sy_id     BIGINT;
    target_id BIGINT;
BEGIN
    SELECT display_brand_id INTO kgm_id FROM catalog_brands WHERE tsb_name = 'KGMOBILITY' LIMIT 1;
    SELECT display_brand_id INTO sy_id FROM catalog_brands WHERE tsb_name = 'SSANGYONG' LIMIT 1;
    target_id := COALESCE(kgm_id, sy_id);
    IF target_id IS NULL THEN
        RETURN;
    END IF;

    UPDATE catalog_display_brands
    SET name = 'KG Mobility (SsangYong)', slug = 'kg-mobility-ssangyong'
    WHERE id = target_id
      AND name IN ('KG Mobility', 'SsangYong')
      AND NOT EXISTS (
          SELECT 1 FROM catalog_display_brands o
          WHERE o.id <> target_id
            AND (lower(o.name) = lower('KG Mobility (SsangYong)') OR o.slug = 'kg-mobility-ssangyong')
      );

    IF sy_id IS NOT NULL AND kgm_id IS NOT NULL AND sy_id <> kgm_id
       AND EXISTS (SELECT 1 FROM catalog_display_brands WHERE id = sy_id AND name = 'SsangYong') THEN
        UPDATE catalog_brands SET display_brand_id = target_id WHERE display_brand_id = sy_id;
        UPDATE catalog_display_brands t
        SET logo_object_key = s.logo_object_key
        FROM catalog_display_brands s
        WHERE t.id = target_id AND s.id = sy_id
          AND t.logo_object_key IS NULL AND s.logo_object_key IS NOT NULL;
        DELETE FROM catalog_display_brands d
        WHERE d.id = sy_id
          AND NOT EXISTS (SELECT 1 FROM catalog_brands b WHERE b.display_brand_id = d.id);
    END IF;

    UPDATE catalog_brands b
    SET display_name = d.name
    FROM catalog_display_brands d
    WHERE b.display_brand_id = target_id AND d.id = target_id AND b.display_name <> d.name;
END $$;
