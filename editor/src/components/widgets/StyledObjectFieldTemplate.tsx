import type { ObjectFieldTemplateProps } from "@rjsf/utils";

export function StyledObjectFieldTemplate(props: ObjectFieldTemplateProps) {
  const { title, properties, fieldPathId } = props;
  const isRoot = fieldPathId.$id === "root";

  // Root-level object: render children without wrapper
  if (isRoot) {
    return (
      <div>
        {properties.map((p) => (
          <div key={p.name}>{p.content}</div>
        ))}
      </div>
    );
  }

  // Nested object: render with styled container
  return (
    <div className="mb-2">
      {title && (
        <label className="text-sm font-medium text-gray-700 block mb-1">
          {title}
        </label>
      )}
      <div className="bg-gray-50 rounded p-3 border border-gray-200 space-y-1">
        {properties.map((p) => (
          <div key={p.name}>{p.content}</div>
        ))}
      </div>
    </div>
  );
}
