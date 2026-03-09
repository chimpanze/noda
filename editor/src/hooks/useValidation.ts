import { useEffect, useRef } from "react";
import { useEditorStore } from "@/stores/editor";
import * as api from "@/api/client";

/**
 * Debounced validation that runs when the active workflow changes.
 */
export function useValidation() {
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const setValidationErrors = useEditorStore((s) => s.setValidationErrors);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!activeWorkflowPath) {
      setValidationErrors([]);
      return;
    }

    if (timerRef.current) clearTimeout(timerRef.current);

    timerRef.current = setTimeout(async () => {
      try {
        const result = await api.validateFile(activeWorkflowPath);
        setValidationErrors(result.errors ?? []);
      } catch {
        // Validation endpoint may not be available, ignore
      }
    }, 500);

    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [activeWorkflow, activeWorkflowPath, setValidationErrors]);
}
