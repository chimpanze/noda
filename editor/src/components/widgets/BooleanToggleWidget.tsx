import { useCallback } from "react";
import type { WidgetProps } from "@rjsf/utils";

export function BooleanToggleWidget(props: WidgetProps) {
  const { value, onChange, label, required, readonly } = props;
  const checked = value === true;

  const toggle = useCallback(() => {
    if (!readonly) onChange(!checked);
  }, [onChange, checked, readonly]);

  return (
    <div className="mb-2">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium text-gray-700">
          {label}
          {required && <span className="text-red-500 ml-0.5">*</span>}
        </label>
        <button
          type="button"
          role="switch"
          aria-checked={checked}
          onClick={toggle}
          disabled={readonly}
          className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
            checked ? "bg-blue-500" : "bg-gray-300"
          } ${readonly ? "opacity-50 cursor-not-allowed" : "cursor-pointer"}`}
        >
          <span
            className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
              checked ? "translate-x-4" : "translate-x-0.5"
            }`}
          />
        </button>
      </div>
    </div>
  );
}
