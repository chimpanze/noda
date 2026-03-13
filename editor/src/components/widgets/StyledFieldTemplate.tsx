import type { FieldTemplateProps } from "@rjsf/utils";

export function StyledFieldTemplate(props: FieldTemplateProps) {
  const { label, children, schema, required, displayLabel } = props;
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
    </div>
  );
}
