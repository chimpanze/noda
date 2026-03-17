import type { FieldTemplateProps } from "@rjsf/utils";
import type { ValidationError } from "@/types";

export function StyledFieldTemplate(props: FieldTemplateProps) {
  const { label, children, schema, required, displayLabel, id, registry } =
    props;
  const formContext = registry?.formContext as
    | { validationErrors?: ValidationError[] }
    | undefined;

  const errors: string[] = [];
  if (formContext?.validationErrors && id) {
    const fieldPath = id.replace(/^root_/, "").replace(/_/g, ".");
    for (const err of formContext.validationErrors) {
      if (err.path === fieldPath || err.path.endsWith("." + fieldPath)) {
        errors.push(err.message);
      }
    }
  }

  return (
    <div className="mb-3">
      {displayLabel && label && (
        <label className="text-sm font-medium text-gray-700 block mb-1">
          {label}
          {required && <span className="text-red-400 ml-0.5">*</span>}
        </label>
      )}
      {children}
      {schema.description && (
        <p className="text-xs text-gray-400 mt-0.5">{schema.description}</p>
      )}
      {errors.map((err, i) => (
        <p key={i} className="text-xs text-red-500 mt-0.5">
          {err}
        </p>
      ))}
    </div>
  );
}
