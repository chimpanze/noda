import { useMemo } from "react";
import { useEditorStore } from "@/stores/editor";

export function useFieldValidation(fieldPath: string): string[] {
  const validationErrors = useEditorStore((s) => s.validationErrors);

  return useMemo(
    () =>
      validationErrors
        .filter(
          (e) => e.path === fieldPath || e.path.startsWith(fieldPath + "."),
        )
        .map((e) => e.message),
    [validationErrors, fieldPath],
  );
}
