# LocationInfo

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Accuracy** | **int32** | Horizontal accuracy/(semi-major) uncertainty of location provided in meters, as defined in [14]. Present only if \&quot;shape\&quot; equals 4, 5 or 6. | [optional] [default to null]
**AccuracyAltitude** | **int32** | Altitude accuracy/uncertainty of location provided in meters, as defined in [14]. Present only if \&quot;shape\&quot; equals 3 or 4. | [optional] [default to null]
**AccuracySemiMinor** | **int32** | Horizontal accuracy/(semi-major) uncertainty of location provided in meters, as defined in [14]. Present only if \&quot;shape\&quot; equals 4, 5 or 6. | [optional] [default to null]
**Altitude** | **float32** | Location altitude relative to the WGS84 ellipsoid surface. | [optional] [default to null]
**Confidence** | **int32** | Confidence by which the position of a target entity is known to be within the shape description, expressed as a percentage and defined in [14]. Present only if \&quot;shape\&quot; equals 1, 4 or 6. | [optional] [default to null]
**IncludedAngle** | **int32** | Present only if \&quot;shape\&quot; equals 6. | [optional] [default to null]
**InnerRadius** | **int32** | Present only if \&quot;shape\&quot; equals 6. | [optional] [default to null]
**Latitude** | **[]float64** | Location latitude, expressed in the range -90° to +90°. Cardinality greater than one only if \&quot;shape\&quot; equals 7. | [default to null]
**Longitude** | **[]float64** | Location longitude, expressed in the range -180° to +180°. Cardinality greater than one only if \&quot;shape\&quot; equals 7. | [default to null]
**OffsetAngle** | **int32** | Present only if \&quot;shape\&quot; equals 6. | [optional] [default to null]
**OrientationMajorAxis** | **int32** | Angle of orientation of the major axis, expressed in the range 0° to 180°, as defined in [14]. Present only if \&quot;shape\&quot; equals 4 or 6. | [optional] [default to null]
**Shape** | **int32** | Shape information, as detailed in [14], associated with the reported location coordinate: 1 &#x3D; Ellipsoid_Arc 2 &#x3D; ellipsoid_Point 3 &#x3D; ellipsoid_Point_Altitude 4 &#x3D; ellipsoid_Point_Altitude_Uncert_Ellipsoid 5 &#x3D; ellipsoid_Point_Uncert_Circle 6 &#x3D; ellipsoid_Point_Uncert_Ellipse 7 &#x3D; polygon | [default to null]
**UncertaintyRadius** | **int32** | Present only if \&quot;shape\&quot; equals 6. | [optional] [default to null]
**Velocity** | [***Velocity**](Velocity.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

